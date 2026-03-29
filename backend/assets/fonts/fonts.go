// Package fonts embeds the bundled font files used for poster overlay rendering.
// Fonts are licensed under the SIL Open Font License — see OFL.txt in this directory.
package fonts

import _ "embed" // Required for //go:embed directives on package-level variables.

// NotoSansBold is the embedded Noto Sans Bold TTF font data.
// Used by the poster overlay package for countdown banner text.
//
//go:embed NotoSans-Bold.ttf
var NotoSansBold []byte
