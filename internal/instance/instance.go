// Package instance keeps to one copy of the program running for each user. A
// copy started while another runs hands that one what it was started with, the
// desktop's leave to take the focus and the folder to open, and leaves, so that
// starting the program again brings back the window there is. It knows nothing
// of the UI.
//
// The copy that runs holds a lock on a file, which the system lets go of if the
// program dies, and listens on a socket beside it. A copy that can't take the
// lock sends the one that has it a Request through the socket.
package instance

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// ErrRunning is returned by Claim when another copy is running and has been
// handed the request.
var ErrRunning = errors.New("another copy of the program is running")

// ErrNoAnswer is returned by Claim when another copy holds the lock but does not
// answer, as one that hangs. This copy must not run beside it.
var ErrNoAnswer = errors.New("another copy of the program is running but does not answer")

// The names of the lock and the socket within the folder.
const (
	lockName   = "delta.lock"
	socketName = "delta.sock"
)

// answerWait is how long a copy started while another runs waits for that one to
// listen, as one that has just started may have the lock but not yet the socket.
// readWait is how long the copy that runs waits for a request once a connection
// is made; it is written at once.
var (
	answerWait = 5 * time.Second
	readWait   = time.Second
)

// maxRequest is the most of a request that is read.
const maxRequest = 64 << 10

// Request is what a copy started while another runs hands it.
type Request struct {
	// Token is the desktop's leave to take the focus, which Wayland needs, and
	// may be empty.
	Token string `json:"token,omitempty"`
	// Folder is an absolute path: the one named on the command line if Named,
	// or else the folder the copy was started in.
	Folder string `json:"folder,omitempty"`
	Named  bool   `json:"named,omitempty"`
}

// Dir returns the folder the lock and the socket are kept in: the user's runtime
// folder (XDG_RUNTIME_DIR), which only they can open and which goes when they log
// out, or else a folder of their own in the temporary folder, which is one for
// all users.
func Dir() (string, error) {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return dir, nil
	}
	dir := filepath.Join(os.TempDir(), fmt.Sprintf("delta-%d", os.Getuid()))
	if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return "", err
	}
	// Anyone can make a folder in a temporary folder for all, so one made before
	// is used only if it is the user's and only theirs.
	info, err := os.Lstat(dir)
	if err != nil {
		return "", err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !info.IsDir() || info.Mode().Perm() != 0o700 || !ok || int(stat.Uid) != os.Getuid() {
		return "", fmt.Errorf("%s is not a folder only this user can open", dir)
	}
	return dir, nil
}

// Instance is the claim of the copy that runs.
type Instance struct {
	lock     *os.File
	listener net.Listener

	// mu guards closed, so that nothing is passed on once Close has returned;
	// conns are the connections being read, closed with it.
	mu     sync.Mutex
	closed bool
	conns  map[net.Conn]bool
	wg     sync.WaitGroup
}

// Claim makes this the copy that runs, with its lock and socket in dir, if no
// other copy is running. If one is, it is handed the request and ErrRunning is
// returned, or ErrNoAnswer if it could not be.
func Claim(dir string, request Request) (*Instance, error) {
	lock, err := os.OpenFile(filepath.Join(dir, lockName), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, socketName)
	data, err := json.Marshal(request)
	if err != nil {
		lock.Close()
		return nil, err
	}
	// Until the copy that runs answers, the lock is tried again, as it may be
	// quitting, its socket gone but its lock not yet let go of.
	deadline := time.Now().Add(answerWait)
	for {
		held, err := tryLock(lock)
		if err != nil {
			lock.Close()
			return nil, err
		}
		if held {
			break
		}
		err = handOver(path, data)
		if err == nil {
			lock.Close()
			return nil, ErrRunning
		}
		if time.Now().After(deadline) {
			lock.Close()
			return nil, fmt.Errorf("%w: %w", ErrNoAnswer, err)
		}
		time.Sleep(50 * time.Millisecond)
	}

	// A socket left by a copy that died is in the way. Only the copy that holds
	// the lock takes it away, so it is never one in use.
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		lock.Close()
		return nil, err
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		lock.Close()
		return nil, err
	}
	return &Instance{lock: lock, listener: listener, conns: map[net.Conn]bool{}}, nil
}

// tryLock takes the lock on the file, and says whether it could: it can't while
// another copy holds it.
func tryLock(f *os.File) (bool, error) {
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return false, nil
	}
	return err == nil, err
}

// handOver sends the request, as JSON, to the copy that runs, if it listens.
func handOver(path string, data []byte) error {
	conn, err := net.Dial("unix", path)
	if err != nil {
		return err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(answerWait))
	_, err = conn.Write(append(data, '\n'))
	return err
}

// Serve calls handle, on a goroutine of its own, with the request of each copy
// started later, until Close. It returns at once.
func (i *Instance) Serve(handle func(Request)) {
	i.wg.Add(1)
	go func() {
		defer i.wg.Done()
		for {
			conn, err := i.listener.Accept()
			if err != nil {
				return
			}
			i.mu.Lock()
			if i.closed {
				i.mu.Unlock()
				conn.Close()
				return
			}
			i.conns[conn] = true
			i.wg.Add(1)
			i.mu.Unlock()
			go i.read(conn, handle)
		}
	}()
}

// read reads the request of a copy started later and passes it on, unless it is
// not one or the instance has been closed meanwhile.
func (i *Instance) read(conn net.Conn, handle func(Request)) {
	defer i.wg.Done()
	defer func() {
		i.mu.Lock()
		delete(i.conns, conn)
		i.mu.Unlock()
		conn.Close()
	}()
	conn.SetReadDeadline(time.Now().Add(readWait))
	line, err := bufio.NewReader(io.LimitReader(conn, maxRequest)).ReadBytes('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return
	}
	var request Request
	if json.Unmarshal(line, &request) != nil {
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if !i.closed {
		handle(request)
	}
}

// Close stops listening and lets go of the claim, waiting for what Serve
// started; handle is not called once it has returned.
func (i *Instance) Close() error {
	i.mu.Lock()
	i.closed = true
	for conn := range i.conns {
		conn.Close()
	}
	i.mu.Unlock()
	err := i.listener.Close()
	i.wg.Wait()
	return errors.Join(err, i.lock.Close())
}
