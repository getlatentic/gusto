package setup

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/getlatentic/gusto/internal/integration"
)

// Export is a skill set as an Agent Plugin, in a zip: a plugin.json and its skills, and no servers. It is a plugin like any other,
// so another deployment can import it as an integration, and this one can too. The servers a setup names are the deployment's own
// and the integrations it names are imported, so neither is in it.
func (s *Store) Export(name string) ([]byte, error) {
	held, found := s.Get(name)
	if !found {
		return nil, fmt.Errorf("no setup called %s", name)
	}
	if len(held.Skills) == 0 {
		return nil, fmt.Errorf("the setup %s has no skills, and a plugin of none is nothing to share", name)
	}
	manifest, err := json.MarshalIndent(map[string]any{
		"$schema":     "https://agent-plugins.org/schemas/" + integration.Format + "/plugin.schema.json",
		"name":        name,
		"description": "The skill set " + name + ".",
	}, "", "  ")
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	archive := zip.NewWriter(&out)
	write := func(path string, text []byte) error {
		file, err := archive.Create(path)
		if err != nil {
			return err
		}
		_, err = file.Write(text)
		return err
	}
	if err := write("plugin.json", append(manifest, '\n')); err != nil {
		return nil, err
	}
	for _, skill := range held.Skills {
		description := strings.Join(strings.Fields(skill.Description), " ")
		if err := write("skills/"+skill.Name+"/SKILL.md", []byte(document(skill.Name, description, skill.Body))); err != nil {
			return nil, err
		}
	}
	if err := archive.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
