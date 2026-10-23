package web

// usageTabs is the two views of one page: the month's totals, and the list of tasks behind them.
type usageTabs struct {
	Summary, Tasks, Current string
}

func tabsFor(console bool, current string) usageTabs {
	if console {
		return usageTabs{Summary: "/admin/spending", Tasks: "/admin/usage", Current: current}
	}
	return usageTabs{Summary: "/spend", Tasks: "/usage", Current: current}
}
