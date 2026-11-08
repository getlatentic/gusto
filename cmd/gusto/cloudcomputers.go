package main

import (
	"context"
	"fmt"
	"github.com/getlatentic/gusto/internal/workspace"
	"io"
	"text/tabwriter"
	"time"

	"github.com/getlatentic/gusto/cmd/internal/cmdline"
	"github.com/getlatentic/gusto/internal/cloud"
)

// listComputers is every computer on this node, with the organization each is in and
// whose it is: the operator's view across organizations, read from each one's own record.
func listComputers(ctx context.Context, w io.Writer, store *cloud.Store) error {
	orgs, err := store.Orgs(ctx)
	if err != nil {
		return err
	}
	table := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "ORGANIZATION\tWORKSPACE\tPERSON\tSTATE\tHOST\tSANDBOX\tLAST ACTIVE")
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
			fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", org.Slug, workspace.Of(each.Workspace), each.User, each.State, each.Host, each.Sandbox, active)
			total++
		}
	}
	if err := table.Flush(); err != nil {
		return err
	}
	fmt.Fprintf(w, "%d computers in %d organizations\n", total, len(orgs))
	return nil
}

func cloudComputersCommand() *cmdline.Command {
	return &cmdline.Command{
		Name: "computers", Summary: "List every computer on this node",
		Description: "Lists every computer on this node, with the organization each is in and whose it is: the operator's " +
			"view across organizations, read from each one's own record.",
		Env:      envCloudDatabase(),
		Examples: []string{"gusto cloud computers"},
		SeeAlso:  []string{"cloud"},
		Run:      none("cloud computers", cloudComputers),
	}
}
