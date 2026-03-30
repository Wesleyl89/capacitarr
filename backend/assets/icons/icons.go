// Package icons embeds pre-rendered Lucide icon PNGs used for poster overlays.
// Icons are white-on-transparent at 24, 48, and 96px sizes. The poster overlay
// package selects the closest size to the banner height and scales as needed.
//
// Source: Lucide Icons (https://lucide.dev) — ISC License.
package icons

import _ "embed" // Required for //go:embed directives on package-level variables.

// Hourglass24 is the Lucide hourglass icon at 24px (white stroke on transparent).
//
//go:embed hourglass-24.png
var Hourglass24 []byte

// Hourglass48 is the Lucide hourglass icon at 48px (white stroke on transparent).
//
//go:embed hourglass-48.png
var Hourglass48 []byte

// Hourglass96 is the Lucide hourglass icon at 96px (white stroke on transparent).
//
//go:embed hourglass-96.png
var Hourglass96 []byte

// ShieldCheck24 is the Lucide shield-check icon at 24px (white stroke on transparent).
//
//go:embed shield-check-24.png
var ShieldCheck24 []byte

// ShieldCheck48 is the Lucide shield-check icon at 48px (white stroke on transparent).
//
//go:embed shield-check-48.png
var ShieldCheck48 []byte

// ShieldCheck96 is the Lucide shield-check icon at 96px (white stroke on transparent).
//
//go:embed shield-check-96.png
var ShieldCheck96 []byte
