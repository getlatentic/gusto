package cmdline

import "github.com/getlatentic/gusto/internal/settings"

// ParamsOf is the settings that declare declares, as a command's help lists them.
func ParamsOf(declare func(*settings.Set)) []Param {
	set := settings.New()
	declare(set)
	var params []Param
	for _, each := range set.Params() {
		params = append(params, Param{Name: each.Name, Text: each.Text})
	}
	return params
}
