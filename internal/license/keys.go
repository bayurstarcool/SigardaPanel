package license

// defaultPublicKeys is the license public key built into every panel
// (base64 ed25519, comma separated for rotation). Its private key is kept
// by the master panel only (panel.sigarda.com). Releases may add keys with
// -ldflags (SIGARDAPANEL_LICENSE_PUBLIC_KEYS) for a rotation.
const defaultPublicKeys = "NANRhc9A+8P0ALHafE6fZQReJDgzQyaSOApdudp09FA="
