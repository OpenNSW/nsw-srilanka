import { useContext } from 'react'
import { AppearanceContext } from './appearanceContextCore'

export function useAppearance() {
  const context = useContext(AppearanceContext)
  if (context === undefined) {
    throw new Error('useAppearance must be used within an AppearanceProvider')
  }
  return context
}
