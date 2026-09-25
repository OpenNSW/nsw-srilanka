import { useContext } from 'react'
import { ColorSchemeContext } from './colorSchemeContextCore'

export function useColorScheme() {
  const context = useContext(ColorSchemeContext)
  if (context === undefined) {
    throw new Error('useColorScheme must be used within a ColorSchemeProvider')
  }
  return context
}
