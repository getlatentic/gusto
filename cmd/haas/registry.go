package main

import (
	"errors"
	"sync/atomic"

	"github.com/getlatentic/haas/internal/harness"
	"github.com/getlatentic/haas/internal/registry"
	"github.com/getlatentic/haas/internal/web"
)

// openRegistry is the organisation's registry of models, kept in HAAS_REGISTRY, which is added to the models
// this deployment's operator configured. It needs a proxy, since the keys are held there, so a deployment with
// no credential of its own has none and its console has no page for it.
//
// What the operator allows an admin is HAAS_REGISTRY_KEYS, the kinds of reference besides a key typed into the
// page that a key may be given as (file, ssm, secretsmanager), and HAAS_REGISTRY_PRIVATE=allow, which lets a
// provider be at a private address. A deployment that hosts other people's organisations sets neither.
//
// The returned function is what the forms offer, read each time a page is drawn, and changed is called after
// each change, once the proxy and the ledger have it.
func openRegistry(way *WayOut, changed func()) (*registry.Registry, func() web.Models, error) {
	dir := setting("HAAS_REGISTRY", "")
	if dir == "" || way.proxy == nil {
		return nil, nil, nil
	}
	kinds := values("HAAS_REGISTRY_KEYS")
	allowed := registry.Rules{Harnesses: harness.Names(), PrivateAddresses: setting("HAAS_REGISTRY_PRIVATE", "") == "allow", KeySources: kinds}
	reg, err := registry.Open(dir, way.platform, allowed, registry.Sources{Allowed: kinds})
	if err != nil {
		return nil, nil, err
	}
	if reg == nil {
		return nil, nil, errors.New("the registry could not be opened")
	}
	var offered atomic.Pointer[web.Models]
	apply := func(e registry.Effective) {
		way.Apply(e)
		models := offeredModels(e.Credentials, e.Models, harness.Names())
		offered.Store(&models)
		changed()
	}
	apply(reg.Effective())
	reg.OnChange(apply)
	return reg, func() web.Models { return *offered.Load() }, nil
}
