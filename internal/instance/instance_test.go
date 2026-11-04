package instance

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// shortDir is a folder for a lock and socket. A socket's path is limited to
// about a hundred bytes, which t.TempDir, named after the test, can pass.
func shortDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "delta")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// claim claims dir, failing the test if it can't, and serves into a channel.
func claim(t *testing.T, dir string) (*Instance, chan Request) {
	t.Helper()
	i, err := Claim(dir, Request{})
	if err != nil {
		t.Fatal(err)
	}
	requests := make(chan Request, 10)
	i.Serve(func(r Request) { requests <- r })
	t.Cleanup(func() { i.Close() })
	return i, requests
}

// receive waits for a request.
func receive(t *testing.T, requests chan Request) Request {
	t.Helper()
	select {
	case r := <-requests:
		return r
	case <-time.After(5 * time.Second):
		t.Fatal("no request was passed on")
		return Request{}
	}
}

// holdLock takes the lock in dir as another copy would.
func holdLock(t *testing.T, dir string) *os.File {
	t.Helper()
	f, err := os.OpenFile(filepath.Join(dir, lockName), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	return f
}

// A copy started while another runs hands it the request whole, and leaves.
func TestSecondCopyHandsOver(t *testing.T) {
	dir := shortDir(t)
	_, requests := claim(t, dir)

	sent := Request{Token: "token-1", Folder: "/a folder/with\nodd names", Named: true}
	if _, err := Claim(dir, sent); !errors.Is(err, ErrRunning) {
		t.Fatalf("second claim = %v", err)
	}
	if got := receive(t, requests); got != sent {
		t.Errorf("got %+v", got)
	}
	// One with nothing to hand over still has the window shown.
	if _, err := Claim(dir, Request{}); !errors.Is(err, ErrRunning) {
		t.Fatalf("third claim = %v", err)
	}
	if got := receive(t, requests); got != (Request{}) {
		t.Errorf("got %+v", got)
	}
}

// Once the copy that ran has closed, the next one runs.
func TestClaimAfterClose(t *testing.T) {
	dir := shortDir(t)
	i, err := Claim(dir, Request{})
	if err != nil {
		t.Fatal(err)
	}
	if err := i.Close(); err != nil {
		t.Fatal(err)
	}
	_, requests := claim(t, dir)
	if _, err := Claim(dir, Request{Token: "x"}); !errors.Is(err, ErrRunning) {
		t.Fatalf("claim = %v", err)
	}
	receive(t, requests)
}

// A socket left by a copy that died, which holds no lock, is taken over rather
// than taken for a copy that runs.
func TestStaleSocket(t *testing.T) {
	dir := shortDir(t)
	listener, err := net.Listen("unix", filepath.Join(dir, socketName))
	if err != nil {
		t.Fatal(err)
	}
	// The socket's file stays, as when its program is killed.
	listener.(*net.UnixListener).SetUnlinkOnClose(false)
	listener.Close()

	_, requests := claim(t, dir)
	if _, err := Claim(dir, Request{Token: "after"}); !errors.Is(err, ErrRunning) {
		t.Fatalf("claim = %v", err)
	}
	if got := receive(t, requests); got.Token != "after" {
		t.Errorf("got %+v", got)
	}
}

// A copy started just after another, which holds the lock but doesn't yet
// listen, waits for it.
func TestHandOverWaitsForListening(t *testing.T) {
	dir := shortDir(t)
	lock := holdLock(t, dir)
	defer lock.Close()

	got := make(chan string, 1)
	go func() {
		time.Sleep(200 * time.Millisecond)
		listener, err := net.Listen("unix", filepath.Join(dir, socketName))
		if err != nil {
			got <- err.Error()
			return
		}
		defer listener.Close()
		conn, err := listener.Accept()
		if err != nil {
			got <- err.Error()
			return
		}
		defer conn.Close()
		buf := make([]byte, 100)
		n, _ := conn.Read(buf)
		got <- string(buf[:n])
	}()

	if _, err := Claim(dir, Request{Token: "late"}); !errors.Is(err, ErrRunning) {
		t.Fatalf("claim = %v", err)
	}
	if s := <-got; s != `{"token":"late"}`+"\n" {
		t.Errorf("sent %q", s)
	}
}

// A copy started while another holds the lock but never listens says so,
// rather than running beside it.
func TestNoAnswer(t *testing.T) {
	old := answerWait
	answerWait = 200 * time.Millisecond
	defer func() { answerWait = old }()

	dir := shortDir(t)
	lock := holdLock(t, dir)
	defer lock.Close()

	i, err := Claim(dir, Request{})
	if !errors.Is(err, ErrNoAnswer) || i != nil {
		t.Fatalf("claim = %v, %v", i, err)
	}
}

// A connection that sends nothing, or something that is no request, holds up
// neither the copies started after it nor Close, and passes nothing on.
func TestIdleAndBadConnections(t *testing.T) {
	dir := shortDir(t)
	i, requests := claim(t, dir)

	idle, err := net.Dial("unix", filepath.Join(dir, socketName))
	if err != nil {
		t.Fatal(err)
	}
	defer idle.Close()
	bad, err := net.Dial("unix", filepath.Join(dir, socketName))
	if err != nil {
		t.Fatal(err)
	}
	bad.Write([]byte("not json\n"))
	bad.Close()

	if _, err := Claim(dir, Request{Token: "next"}); !errors.Is(err, ErrRunning) {
		t.Fatalf("claim = %v", err)
	}
	if got := receive(t, requests); got.Token != "next" {
		t.Errorf("got %+v", got)
	}

	start := time.Now()
	if err := i.Close(); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took > readWait/2 {
		t.Errorf("Close took %v", took)
	}
	if len(requests) != 0 {
		t.Errorf("passed on %+v", <-requests)
	}
}

// Without a runtime folder, one of the user's own in the temporary folder is
// used, but not one others can open.
func TestDirFallback(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("TMPDIR", tmp)

	dir, err := Dir()
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(dir) != tmp || info.Mode().Perm() != 0o700 {
		t.Errorf("dir = %s, %v", dir, info.Mode())
	}
	if again, err := Dir(); err != nil || again != dir {
		t.Errorf("again = %s, %v", again, err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Dir(); err == nil {
		t.Error("a folder others can open was used")
	}
}

func TestDirRuntime(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	if dir, err := Dir(); err != nil || dir != "/run/user/1000" {
		t.Errorf("dir = %s, %v", dir, err)
	}
}
