import { MailIcon, UserIcon } from "lucide-react"

import { AppLink } from "@/components/app-link"
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import { emailWords } from "@/domain/members"
import { AgentFace, PersonFace } from "@/pages/members/faces"
import {
  BackToMembers,
  Fact,
  Facts,
  RoomLinks,
  sectionTitle,
} from "@/pages/members/member-parts"
import type { ListedAgent, PersonData } from "@/pages/members/types"
import { useMembersForm } from "@/pages/members/use-members-form"

// PersonView is one person: who they are, the bio they wrote, the rooms the person looking shares with them, and the agents
// they made.
export function PersonView({ page }: { page: PersonData }) {
  const person = page.Person
  return (
    <article
      aria-label={person.Name}
      className="mx-auto grid max-w-3xl gap-8 px-8 py-8 max-nav:px-4"
      data-testid="person-page"
    >
      <BackToMembers />
      <header className="flex items-center gap-4">
        <PersonFace initials={person.Initials} size="lg" />
        <div className="grid min-w-0">
          <h2 className="m-0 truncate text-base font-semibold">
            {person.Name}
          </h2>
          <p className="m-0 truncate text-sm text-subtle">{person.Role}</p>
        </div>
      </header>
      <section aria-label="Profile" className="grid gap-3">
        <h3 className={sectionTitle}>Profile</h3>
        <Facts>
          <Fact
            term={
              <>
                <MailIcon aria-hidden="true" /> Email
              </>
            }
          >
            {emailWords(person)}
          </Fact>
          <Fact
            term={
              <>
                <UserIcon aria-hidden="true" /> Role
              </>
            }
          >
            {person.Role}
          </Fact>
        </Facts>
      </section>
      <PersonBio page={page} />
      <RoomLinks title="Rooms you share" rooms={page.Rooms} />
      <MadeBy agents={page.Made} />
    </article>
  )
}

// PersonBio is what the person wrote of themselves, which they change here when it is theirs.
function PersonBio({ page }: { page: PersonData }) {
  const { submit, failed, sending } = useMembersForm()
  const bio = page.Bio
  return (
    <section aria-label="Bio" className="grid gap-3">
      <h3 className={sectionTitle}>Bio</h3>
      {!bio.Mine &&
        (bio.Text ? (
          <p className="m-0 text-sm leading-relaxed wrap-anywhere whitespace-pre-wrap">
            {bio.Text}
          </p>
        ) : (
          <p className="m-0 text-sm text-subtle">Nothing written yet.</p>
        ))}
      {bio.Mine && (
        <form
          method="post"
          action={`${page.Person.Where}/bio`}
          onSubmit={submit}
          className="m-0 grid gap-3"
          data-testid="bio-form"
        >
          {(page.Refused || failed) && (
            <p
              role="alert"
              className="m-0 rounded-md bg-bad-wash px-4 py-3 text-sm text-bad"
            >
              {page.Refused || failed}
            </p>
          )}
          <p id="bio-note" className="m-0 text-sm text-muted-foreground">
            What you do, what you are working on, how you like to work. The
            agents in your rooms read it, and so does everybody who opens this
            page.
          </p>
          <Textarea
            key={bio.Text}
            name="bio"
            aria-label="Your bio"
            aria-describedby="bio-note"
            maxLength={bio.Max}
            defaultValue={bio.Text}
            className="min-h-24 resize-y leading-relaxed"
            placeholder="For example: I run launches. This month: the pricing page. I like short answers, with the source."
          />
          <div className="flex items-center gap-2">
            <span className="mr-auto text-xs text-subtle">
              Up to {bio.Max} characters
            </span>
            <Button type="submit" disabled={sending}>
              Save
            </Button>
          </div>
        </form>
      )}
    </section>
  )
}

function MadeBy({ agents }: { agents: ListedAgent[] }) {
  if (agents.length === 0) return null
  return (
    <section aria-label="Agents they made" className="grid gap-3">
      <h3 className={sectionTitle}>Agents they made</h3>
      <ul className="m-0 grid list-none gap-px rounded-lg border p-1.5">
        {agents.map((agent) => (
          <li key={agent.ID}>
            <AppLink
              href={agent.Where}
              className="flex h-10 items-center gap-3 rounded-md px-2 text-sm text-foreground no-underline hover:bg-hover"
            >
              <AgentFace mark={agent.Mark} name={agent.Name} size="sm" />
              {agent.Name}
              {agent.Specialty && (
                <span className="text-subtle">{agent.Specialty}</span>
              )}
            </AppLink>
          </li>
        ))}
      </ul>
    </section>
  )
}
