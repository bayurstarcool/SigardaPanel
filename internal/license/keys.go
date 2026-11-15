package license

// defaultPublicKeys is the license public key built into every panel
// (base64 ed25519, comma separated for rotation). Its private key is kept
// by the master panel only (panel.sigarda.com). Releases may add keys with
// -ldflags (SIGARDAPANEL_LICENSE_PUBLIC_KEYS) for a rotation.
const defaultPublicKeys = "TXXj6yjbwi7nb6ZO2KN3AoI1hMpeW/MaVBWFq/LKhgM="
