package vmwall

import (
	"os"
	"path/filepath"
)

// Remove stops the gateway and takes away everything Apply made: the programs, the image, the certificates, the gateway's
// working folder and the record the openshell command kept of the gateway.
func Remove(l Layout, configHome string) error {
	if err := stopGateway(l); err != nil {
		return err
	}
	for _, dir := range []string{
		l.Root, l.State(),
		filepath.Join(configHome, "openshell", "gateways", GatewayName),
	} {
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
	}
	return nil
}
