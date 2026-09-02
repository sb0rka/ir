import { Moon, Sun } from 'lucide-react'
import { Button } from './ui'
import { useTheme } from './theme-provider'

export function ThemeToggle() {
  const { setTheme, resolvedAppearance } = useTheme()
  const isDark = resolvedAppearance === 'dark'

  return (
    <Button
      size="icon"
      variant="ghost"
      title="Переключить тему"
      aria-label="Переключить тему"
      onClick={() => setTheme(isDark ? 'light' : 'dark')}
    >
      <span className="relative flex h-3.5 w-3.5 items-center justify-center">
        <Sun
          className={`absolute h-3.5 w-3.5 transition-opacity duration-200 ${
            isDark ? 'opacity-0' : 'opacity-100'
          }`}
        />
        <Moon
          className={`absolute h-3.5 w-3.5 transition-opacity duration-200 ${
            isDark ? 'opacity-100' : 'opacity-0'
          }`}
        />
      </span>
    </Button>
  )
}
