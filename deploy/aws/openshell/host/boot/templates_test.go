package boot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// task is what of an Ansible task names a file of the role: the template or copy it runs, and the items it loops over.
type task struct {
	Template map[string]any `yaml:"ansible.builtin.template"`
	Copy     map[string]any `yaml:"ansible.builtin.copy"`
	Loop     any            `yaml:"loop"`
}

// Every template and file the cloud role's tasks name is in the role, so a name that a rename changed on one side only
// is found here and not when a host fails to start. Ansible's syntax check does not look.
func TestEveryFileTheCloudRoleNamesIsInTheRole(t *testing.T) {
	role := filepath.Join("..", "..", "ansible", "roles", "cloud")
	books, _ := filepath.Glob(filepath.Join(role, "tasks", "*.yml"))
	if len(books) == 0 {
		t.Fatal("the role has no tasks")
	}
	for _, book := range books {
		var tasks []task
		if err := yaml.Unmarshal([]byte(mustRead(t, book)), &tasks); err != nil {
			t.Fatalf("%s: %v", book, err)
		}
		for _, each := range tasks {
			for _, name := range named(each.Template, each.Loop) {
				if _, err := os.Stat(filepath.Join(role, "templates", name)); err != nil {
					t.Errorf("%s names the template %s, which the role does not have", filepath.Base(book), name)
				}
			}
			for _, name := range named(each.Copy, each.Loop) {
				if _, err := os.Stat(filepath.Join(role, "files", name)); err != nil {
					t.Errorf("%s names the file %s, which the role does not have", filepath.Base(book), name)
				}
			}
		}
	}
}

// named is the files of the role a template or copy task reads: its src, or each item's when it loops over a list. A src
// outside the role, or one made some other way, such as by a loop over an expression, is left out.
func named(module map[string]any, loop any) []string {
	src, _ := module["src"].(string)
	if src == "" || module["remote_src"] == true {
		return nil
	}
	if loop == nil {
		return inRole([]string{src})
	}
	items, _ := loop.([]any)
	var names []string
	for _, item := range items {
		switch v := item.(type) {
		case string:
			names = append(names, strings.ReplaceAll(src, "{{ item }}", v))
		case map[string]any:
			if itemSrc, ok := v["src"].(string); ok && src == "{{ item.src }}" {
				names = append(names, itemSrc)
			}
		}
	}
	return inRole(names)
}

// inRole is the names that are of files in the role, and not made by Ansible or outside it.
func inRole(names []string) []string {
	var kept []string
	for _, name := range names {
		if !strings.Contains(name, "{{") && !filepath.IsAbs(name) {
			kept = append(kept, name)
		}
	}
	return kept
}
