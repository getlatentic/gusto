package web

import (
	"fmt"

	"github.com/getlatentic/gusto/internal/catalog"
)

// measuredView is what a matrix saw each engine do with each model on the
// simplest task it ran: the grid a person reads to choose an engine. A model
// reached through two credentials is one column, since it is one model.
type measuredView struct {
	Models   []string
	Rows     []engineRow
	Finished int
	Tried    int
}

// engineRow is one agent type's line of the grid, with its mark.
type engineRow struct {
	Name, Title, Mark, MarkDark string
	Cells                       []measuredCell
}

// measuredCell is one engine on one model. Tried is false where no matrix
// ran the pair, which is not the same as the pair failing.
type measuredCell struct {
	Tried    bool
	Finished bool
	Took     string
	Cost     string
}

func measuredOf(entries []catalog.Entry) measuredView {
	var view measuredView
	column := map[string]int{}
	for _, entry := range entries {
		for _, run := range entry.Measured {
			if _, seen := column[modelOf(run.Model)]; !seen {
				column[modelOf(run.Model)] = len(view.Models)
				view.Models = append(view.Models, modelOf(run.Model))
			}
		}
	}
	for _, entry := range entries {
		if len(entry.Measured) == 0 {
			continue
		}
		row := engineRow{Name: entry.Name, Title: entry.Title, Mark: logo(entry.Name), MarkDark: logoDark(entry.Name), Cells: make([]measuredCell, len(view.Models))}
		for _, run := range entry.Measured {
			row.Cells[column[modelOf(run.Model)]] = cellOf(run)
			view.Tried++
			if run.Finished {
				view.Finished++
			}
		}
		view.Rows = append(view.Rows, row)
	}
	return view
}

func cellOf(run catalog.Measured) measuredCell {
	cell := measuredCell{Tried: true, Finished: run.Finished}
	if run.WallSeconds != nil {
		cell.Took = fmt.Sprintf("%.0f s", *run.WallSeconds)
	}
	if run.CostUSD != nil {
		cell.Cost = fmt.Sprintf("$%.4f", *run.CostUSD)
	}
	return cell
}
