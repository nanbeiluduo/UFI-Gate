// Offline Ed25519 signing helper. Private keys are never embedded in binaries or assets.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	generate := flag.String("generate", "", "new key output directory (keep private)")
	public := flag.Bool("public", false, "print public key only")
	manifest := flag.String("manifest", "", "manifest to sign")
	verify := flag.String("verify", "", "manifest to verify")
	key := flag.String("key", "", "base64 public key for verification")
	flag.Parse()
	must := func(e error) {
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(1)
		}
	}
	if *generate != "" {
		pub, priv, e := ed25519.GenerateKey(rand.Reader)
		must(e)
		must(os.MkdirAll(*generate, 0700))
		for n, b := range map[string][]byte{"update-signing-key.txt": priv.Seed(), "update-public-key.txt": pub} {
			f, e := os.OpenFile(filepath.Join(*generate, n), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			must(e)
			_, e = f.WriteString(base64.StdEncoding.EncodeToString(b) + "\n")
			must(e)
			must(f.Close())
		}
		fmt.Println("Key files created. Store the seed as GitHub Actions secret UPDATE_SIGNING_KEY; do not commit it.")
		return
	}
	if *verify != "" {
		b, e := os.ReadFile(*verify)
		must(e)
		s, e := os.ReadFile(*verify + ".sig")
		must(e)
		sig, e := base64.StdEncoding.DecodeString(string(s))
		must(e)
		pub, e := base64.StdEncoding.DecodeString(*key)
		must(e)
		if len(pub) != ed25519.PublicKeySize || !ed25519.Verify(pub, b, sig) {
			must(fmt.Errorf("invalid signature"))
		}
		return
	}
	seed, e := base64.StdEncoding.DecodeString(os.Getenv("UPDATE_SIGNING_KEY"))
	if e != nil || len(seed) != ed25519.SeedSize {
		must(fmt.Errorf("UPDATE_SIGNING_KEY must be a base64 Ed25519 32-byte seed"))
	}
	priv := ed25519.NewKeyFromSeed(seed)
	if *public {
		fmt.Println(base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey)))
		return
	}
	if *manifest == "" {
		must(fmt.Errorf("missing --manifest"))
	}
	b, e := os.ReadFile(*manifest)
	must(e)
	must(os.WriteFile(*manifest+".sig", []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(priv, b))), 0644))
}
