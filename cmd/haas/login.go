package main

import "github.com/getlatentic/haas/internal/helper/cli"

func login(args []string) int  { return cli.Login(args) }
func logout(args []string) int { return cli.Logout(args) }
func whoami(args []string) int { return cli.Whoami(args) }
