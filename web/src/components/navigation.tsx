import { createContext, useContext } from "react"

// The side list is beside the page on a wide window and slides over it on a narrow one; a page's bar opens it there.
export const NavigationContext = createContext<{ open: () => void }>({
  open: () => {},
})

export function useNavigation() {
  return useContext(NavigationContext)
}
