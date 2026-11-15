import { markLook } from "@/domain/agent-marks"
import { cn } from "@/lib/utils"

// AgentIcon is an agent's mark, drawn for the page's theme.
export function AgentIcon({
  src,
  className,
}: {
  src: string
  className?: string
}) {
  const look = markLook(src)
  const size = cn("flex-none object-contain", className ?? "size-5")
  return (
    <span className="inline-flex flex-none" aria-hidden="true">
      {look.dark ? (
        <>
          <img className={cn(size, "dark:hidden")} src={look.light} alt="" />
          <img
            className={cn(size, "hidden dark:block")}
            src={look.dark}
            alt=""
          />
        </>
      ) : (
        <img
          className={cn(
            size,
            look.invert && "dark:invert",
            look.blend && "mix-blend-multiply dark:mix-blend-screen"
          )}
          src={look.light}
          alt=""
        />
      )}
    </span>
  )
}
