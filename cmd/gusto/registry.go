package main

import (
	"fmt"
	"path/filepath"

	"errors"
	"sync/atomic"

	"github.com/getlatentic/gusto/internal/keychain"

	"github.com/getlatentic/gusto/internal/registry"
	"github.com/getlatentic/gusto/internal/web"
)

// openRegistry is the organization's registry of models, kept in GUSTO_REGISTRY, which is added to the models
// this deployment's operator configured. It needs a proxy, since the keys are held there, so a deployment with
// no credential of its own has none and its console has no page for it.
//
// What the operator allows an admin is GUSTO_REGISTRY_KEYS, the kinds of reference besides a key typed into the
// page that a key may be given as (file, ssm, secretsmanager), and GUSTO_REGISTRY_PRIVATE=allow, which lets a
// provider be at a private address. A deployment that hosts other people's organizations sets neither.
//
// The returned function is what the forms offer, read each time a page is drawn, and changed is called after
// each change, once the proxy and the ledger have it.
//
// fronted says an organization's gusto cloud sends people here, and local that the server is a person's own.
func openRegistry(way *WayOut, fronted, local bool, changed func()) (*registry.Registry, func() web.Models, error) {
	dir := way.cfg.registry
	if dir == "" || way.proxy == nil {
		return nil, nil, nil
	}
	allowed := registry.Rules{PrivateAddresses: way.cfg.registryPrivate == "allow", KeySources: way.cfg.registryKeys}
	reg, err := registry.Open(dir, way.platform, allowed, !fronted)
	if err != nil {
		return nil, nil, err
	}
	if reg == nil {
		return nil, nil, errors.New("the registry could not be opened")
	}
	if local {
		keepKeysInKeychain(reg, dir, way.cfg.keychain == "on")
	}
	var offered atomic.Pointer[web.Models]
	apply := func(e registry.Effective) {
		way.Apply(e)
		models := web.Models(e.Models)
		offered.Store(&models)
		changed()
	}
	apply(reg.Effective())
	reg.OnChange(apply)
	return reg, func() web.Models { return *offered.Load() }, nil
}

// keepKeysInKeychain has a person's own gusto keep the keys they type in their computer's keychain. A computer whose keychain
// cannot be used now, or a person who set GUSTO_KEYCHAIN=off, keeps them in files only this person can read, as a shared
// server does, and says so.
func keepKeysInKeychain(reg *registry.Registry, dir string, keychainOn bool) {
	if !keychainOn {
		fmt.Printf("keys are kept in %s, readable by you alone, since GUSTO_KEYCHAIN is off\n", filepath.Join(dir, "keys"))
		return
	}
	store, err := keychain.Open()
	if err != nil {
		fmt.Printf("keys are kept in %s, readable by you alone, since %v\n", filepath.Join(dir, "keys"), err)
		return
	}
	reg.UseKeys(store)
	fmt.Println("keys are kept in this computer's keychain")
}
