// Command licensetool creates the license signing key and issues and checks
// SigardaPanel license keys. Issuing needs the private key, which only the
// master panel (the seller) holds; see docs/LICENSING.md.
//
//	licensetool keygen -out license.key         # prints the public key
//	licensetool issue -key license.key -id lic_1 -licensee "Acme" -tier pro -servers 10 -days 365
//	licensetool verify -pub <public key> <license key>
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"sigardapanel/internal/license"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "keygen":
		err = keygen(os.Args[2:])
	case "issue":
		err = issue(os.Args[2:])
	case "verify":
		err = verify(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: licensetool keygen|issue|verify [flags] (-h for flags)")
	os.Exit(2)
}

func keygen(args []string) error {
	fs := flag.NewFlagSet("keygen", flag.ExitOnError)
	out := fs.String("out", "license.key", "where to write the private key (keep it secret)")
	fs.Parse(args)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(*out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(f, base64.StdEncoding.EncodeToString(priv.Seed())); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "private key written to %s (mode 0600): keep it secret, never commit it\n", *out)
	fmt.Println(base64.StdEncoding.EncodeToString(pub))
	return nil
}

func readPrivate(path string) (ed25519.PrivateKey, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("%s is not a licensetool private key", path)
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

func issue(args []string) error {
	fs := flag.NewFlagSet("issue", flag.ExitOnError)
	keyPath := fs.String("key", "license.key", "private key file")
	id := fs.String("id", "", "license id (unique, e.g. the order number)")
	licensee := fs.String("licensee", "", "customer name")
	email := fs.String("email", "", "customer email (optional)")
	tier := fs.String("tier", "pro", "basic, pro, enterprise or max")
	servers := fs.Int("servers", 0, "max servers (0 = tier default)")
	days := fs.Int("days", 365, "valid for this many days (0 = never expires)")
	fs.Parse(args)
	priv, err := readPrivate(*keyPath)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Truncate(time.Second)
	c := license.Claims{ID: *id, Licensee: *licensee, Email: *email, Tier: *tier, MaxServers: *servers, IssuedAt: now}
	if *days > 0 {
		c.ExpiresAt = now.AddDate(0, 0, *days)
	}
	key, err := license.Sign(priv, c)
	if err != nil {
		return err
	}
	fmt.Println(key)
	return nil
}

func verify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	pub := fs.String("pub", "", "public key (default: the one built into this binary)")
	fs.Parse(args)
	if fs.NArg() != 1 {
		return fmt.Errorf("give the license key as the argument")
	}
	if *pub != "" {
		license.PublicKeys = *pub
	}
	c, err := license.Verify(fs.Arg(0), time.Now())
	if c != nil {
		out, _ := json.MarshalIndent(c, "", "  ")
		fmt.Println(string(out))
	}
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "valid")
	return nil
}
