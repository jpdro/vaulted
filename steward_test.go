package main

import (
	"bytes"
	"encoding/base64"
	"io"
	"math/rand"
	"os"
	"time"

	"github.com/miquella/vaulted/lib"
	"github.com/miquella/vaulted/lib/legacy"
)

func CaptureStdout(f func()) []byte {
	// Save/restore stdout
	stdout := os.Stdout
	defer func() {
		os.Stdout = stdout
	}()

	// Capture stdout
	r, w, _ := os.Pipe()
	defer w.Close()
	os.Stdout = w

	captured := make(chan []byte)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		captured <- buf.Bytes()
		close(captured)
	}()

	f()

	w.Close()
	return <-captured
}

func WriteStdin(b []byte, f func()) {
	// Save/restore stdin
	stdin := os.Stdin
	defer func() {
		os.Stdin = stdin
	}()

	// Write to stdin
	r, w, _ := os.Pipe()
	defer r.Close()
	os.Stdin = r
	go func() {
		vr := bytes.NewReader(b)
		io.Copy(w, vr)
		w.Close()
	}()

	f()
}

func NewTestSteward() *TestSteward {
	return &TestSteward{
		Passwords: make(map[string]string),
		Vaults:    make(map[string]*vaulted.Vault),
		Sessions:  make(map[string]*vaulted.Session),
	}
}

type TestSteward struct {
	Passwords map[string]string
	Vaults    map[string]*vaulted.Vault
	Sessions  map[string]*vaulted.Session

	LegacyPassword     string
	LegacyEnvironments map[string]legacy.Environment
}

func (ts TestSteward) VaultExists(name string) bool {
	_, exists := ts.Vaults[name]
	return exists
}

func (ts TestSteward) ListVaults() ([]string, error) {
	var vaults []string
	for name := range ts.Vaults {
		vaults = append(vaults, name)
	}
	return vaults, nil
}

func (ts TestSteward) SealVault(name string, password *string, vault *vaulted.Vault) error {
	if password == nil {
		b := make([]byte, 6)
		_, err := rand.Read(b)
		if err != nil {
			return err
		}

		newPassword := base64.StdEncoding.EncodeToString(b)
		password = &newPassword
	}

	ts.Passwords[name] = *password
	ts.Vaults[name] = cloneVault(vault)

	return nil
}

func (ts TestSteward) OpenVault(name string, password *string) (string, *vaulted.Vault, error) {
	if !ts.VaultExists(name) {
		return "", nil, os.ErrNotExist
	}

	if password != nil {
		if ts.Passwords[name] != *password {
			return "", nil, vaulted.ErrInvalidPassword
		}
	}

	return ts.Passwords[name], cloneVault(ts.Vaults[name]), nil
}

func (ts TestSteward) RemoveVault(name string) error {
	if !ts.VaultExists(name) {
		return os.ErrNotExist
	}

	delete(ts.Passwords, name)
	delete(ts.Vaults, name)

	return nil
}

func (ts TestSteward) GetSession(name string, password *string) (string, *vaulted.Session, error) {
	if !ts.VaultExists(name) {
		return "", nil, os.ErrNotExist
	}

	if password != nil {
		if ts.Passwords[name] != *password {
			return "", nil, vaulted.ErrInvalidPassword
		}
	}

	s := &vaulted.Session{
		Expiration: time.Unix(1136239445, 0),
		Vars:       make(map[string]string),
		SSHKeys:    make(map[string]string),
	}
	if _, exists := ts.Sessions[name]; exists {
		cachedSession := ts.Sessions[name]

		s.Name = cachedSession.Name
		s.Expiration = cachedSession.Expiration

		creds := *cachedSession.AWSCreds
		s.AWSCreds = &creds

		for key, value := range cachedSession.Vars {
			s.Vars[key] = value
		}

		for key, value := range cachedSession.SSHKeys {
			s.SSHKeys[key] = value
		}
	} else {
		vault := ts.Vaults[name]

		s.Name = name

		for key, value := range vault.Vars {
			s.Vars[key] = value
		}

		for key, value := range vault.SSHKeys {
			s.SSHKeys[key] = value
		}
	}

	return ts.Passwords[name], s, nil
}

func (ts TestSteward) OpenLegacyVault() (password string, environments map[string]legacy.Environment, err error) {
	return ts.LegacyPassword, ts.LegacyEnvironments, nil
}

func cloneVault(vault *vaulted.Vault) *vaulted.Vault {
	newVault := &vaulted.Vault{
		Vars:    make(map[string]string),
		SSHKeys: make(map[string]string),
	}

	if vault.AWSKey != nil {
		newVault.AWSKey = &vaulted.AWSKey{
			AWSCredentials: vaulted.AWSCredentials{
				ID:     vault.AWSKey.ID,
				Secret: vault.AWSKey.Secret,
			},
			MFA:  vault.AWSKey.MFA,
			Role: vault.AWSKey.Role,
		}
	}

	for key, value := range vault.Vars {
		newVault.Vars[key] = value
	}

	for key, value := range vault.SSHKeys {
		newVault.SSHKeys[key] = value
	}

	return newVault
}
