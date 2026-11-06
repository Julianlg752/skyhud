package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Julianlg752/skyhud/internal/secret"
)

// The key store and the stdin to read a key from; tests swap them.
var (
	keyStore secret.Store = secret.System
	keyInput io.Reader    = os.Stdin
)

// keyCommand stores the DJI API key ("skyhud key set", reading one line from
// standard input) or says whether one is stored ("skyhud key status"). It
// never prints the key.
func keyCommand(args []string, stdout io.Writer) error {
	if len(args) != 1 {
		return errors.New("usage: skyhud key set|status")
	}
	switch args[0] {
	case "set":
		line, err := bufio.NewReader(keyInput).ReadString('\n')
		if err != nil && err != io.EOF {
			return errors.New("could not read the key from standard input")
		}
		key := strings.TrimSpace(line)
		if key == "" {
			return errors.New("no key given on standard input")
		}
		if err := keyStore.Set(secret.DJIKey, key); err != nil {
			return err
		}
		fmt.Fprintln(stdout, "DJI key: stored")
	case "status":
		_, err := keyStore.Get(secret.DJIKey)
		switch {
		case err == nil:
			fmt.Fprintln(stdout, "DJI key: stored")
		case errors.Is(err, secret.ErrNotFound):
			fmt.Fprintln(stdout, "DJI key: not stored")
		default:
			return err
		}
	default:
		return errors.New("usage: skyhud key set|status")
	}
	return nil
}
