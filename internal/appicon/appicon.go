// Package appicon holds the program's icon, built into the binary so that the
// program needs nothing beside it. It knows nothing of Qt.
package appicon

import _ "embed"

// Size is how many pixels wide and high the icon is.
const Size = 256

// PNG is the program's icon, a Size×Size PNG.
//
//go:embed appicon.png
var PNG []byte
