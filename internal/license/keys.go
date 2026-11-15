package license

// defaultPublicKeys is the license public key built into every panel
// (base64 ed25519, comma separated for rotation). Its private key is kept
// by the master panel only. Empty means this build cannot verify licenses
// and runs as the free tier; releases can also set it with -ldflags.
const defaultPublicKeys = ""
