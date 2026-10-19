package main

import (
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/getlatentic/gusto/internal/cloud"
)

// listComputers is every workspace on this host, with the organisation each is in and
// whose it is: the operator's view across organisations, read from each one's own record.
func listComputers(w io.Writer, store *cloud.Store) error {
	orgs, err := store.Orgs()
	if err != nil {
		return err
	}
	table := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "ORGANISATION\tPERSON\tSTATE\tHOST\tSANDBOX\tLAST ACTIVE")
	total := 0
	for _, org := range orgs {
		registry, err := store.Computers(org.Slug)
		if err != nil {
			return err
		}
		all, err := registry.All()
		if err != nil {
			return err
		}
		for _, each := range all {
			active := "never"
			if !each.LastActive.IsZero() {
				active = each.LastActive.Format(time.DateTime)
			}
			fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\t%s\n", org.Slug, each.User, each.State, each.Host, each.Sandbox, active)
			total++
		}
	}
	if err := table.Flush(); err != nil {
		return err
	}
	fmt.Fprintf(w, "%d workspaces in %d organisations\n", total, len(orgs))
	return nil
}
