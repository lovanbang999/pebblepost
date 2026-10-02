export {}

declare global {
  interface Window {
    go?: {
      main?: {
        App?: {
          SelectDirectory?: () => Promise<string>
          WindowMinimise?: () => void
          WindowToggleMaximise?: () => void
          WindowClose?: () => void
        }
      }
    }
  }
}
