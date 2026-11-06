package secret

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// system uses the security command. Values go in through standard input
// (security -i), so they never show up in a process list.
type system struct{}

func (system) Get(name string) (string, error) {
	var out bytes.Buffer
	cmd := exec.Command("security", "find-generic-password", "-s", service, "-a", name, "-w")
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 44 {
			return "", ErrNotFound
		}
		return "", errors.New("keychain: could not read the key")
	}
	return strings.TrimRight(out.String(), "\n"), nil
}

func (system) Set(name, value string) error {
	if strings.ContainsAny(value, "\n\r") {
		return errors.New("keychain: the key must be one line")
	}
	cmd := exec.Command("security", "-i")
	cmd.Stdin = strings.NewReader(fmt.Sprintf("add-generic-password -U -s %s -a %s -w %s\n", quote(service), quote(name), quote(value)))
	if err := cmd.Run(); err != nil {
		return errors.New("keychain: could not store the key")
	}
	return nil
}

func (system) Delete(name string) error {
	if err := exec.Command("security", "delete-generic-password", "-s", service, "-a", name).Run(); err != nil {
		return errors.New("keychain: could not delete the key")
	}
	return nil
}

// quote wraps a value for security's interactive command line.
func quote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}
