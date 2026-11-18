// The demo form says what it is for to whoever sees it: a visitor trying gusto for themselves, or, where only the team gives
// demos, someone of the team starting one for a call with another company.

export type DemoFormWords = {
  title: string
  intro: string
  email: string
  company: string
  submit: string
  // offersCall says the form offers to book a call instead, which only a visitor needs.
  offersCall: boolean
}

export function demoFormWords(page: {
  ForTeam: boolean
  WorkOnly: boolean
  DemoLeft: string
}): DemoFormWords {
  if (page.ForTeam) {
    return {
      title: "Start a demo",
      intro: `For a call: an organization with agents at work, ready in about a minute and deleted after ${page.DemoLeft}. Give your own work email and the company you are showing it to.`,
      email: "Your work email",
      company: "Their company",
      submit: "Start the demo",
      offersCall: false,
    }
  }
  return {
    title: `Try gusto free for ${page.DemoLeft}`,
    intro:
      "Run ready-to-use agents like Claude Code and Codex, and see how gusto gives every employee a private cloud computer where their agents work, with built-in controls for models, spending and security.",
    email: page.WorkOnly ? "Work email" : "Email",
    company: "Company",
    submit: "Start my demo",
    offersCall: true,
  }
}
