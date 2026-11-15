import { useState, type FormEvent } from "react"
import { useNavigate } from "react-router"

import { Loading } from "@/components/controls"
import { Notice, PageBar, PageBody, PageSub } from "@/components/page"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { usePageData, useRefreshAll } from "@/lib/api"
import { sendForm } from "@/lib/forms"

interface NewRoomData {
  Refused: string
}

// NewRoomPage makes a room: a chat that people and agents share, with one folder they all work in. A made room opens with its
// dialog for adding members.
export function NewRoomPage() {
  const loaded = usePageData<NewRoomData>()
  const navigate = useNavigate()
  const refreshAll = useRefreshAll()
  const [making, setMaking] = useState(false)
  const make = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setMaking(true)
    try {
      const landed = await sendForm(event.currentTarget)
      refreshAll()
      navigate(landed)
    } finally {
      setMaking(false)
    }
  }
  return (
    <>
      <PageBar title="New room" />
      <PageBody className="max-w-2xl">
        <PageSub>
          A room is a chat that people and agents share, with one folder they
          all work in.
        </PageSub>
        <Loading loaded={loaded}>
          {(page) => (
            <>
              {page.Refused && <Notice>{page.Refused}</Notice>}
              <Card className="p-6">
                <form
                  method="post"
                  action="/rooms"
                  onSubmit={make}
                  className="grid gap-4"
                  data-testid="new-room-form"
                >
                  <div className="grid gap-2">
                    <Label htmlFor="room-name">Name it</Label>
                    <Input
                      id="room-name"
                      name="name"
                      placeholder="Launch plan"
                      maxLength={60}
                      required
                      autoFocus
                      data-testid="room-name"
                    />
                  </div>
                  <div>
                    <Button
                      type="submit"
                      disabled={making}
                      data-testid="make-room"
                    >
                      Make the room
                    </Button>
                  </div>
                </form>
              </Card>
            </>
          )}
        </Loading>
      </PageBody>
    </>
  )
}
