import { Component, type ReactNode, type ErrorInfo } from 'react'
import { Button, Text } from '@radix-ui/themes'
import { logger } from '../utils/logger'

interface Props {
  children: ReactNode
  fallback?: ReactNode
}

interface State {
  hasError: boolean
  error: Error | null
}

/**
 * Error boundary to catch and handle React errors gracefully
 */
export class ErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props)
    this.state = { hasError: false, error: null }
  }

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error }
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo): void {
    logger.error('Error boundary caught an error:', error, errorInfo)
  }

  handleReset = (): void => {
    this.setState({ hasError: false, error: null })
  }

  render(): ReactNode {
    if (this.state.hasError) {
      if (this.props.fallback) {
        return this.props.fallback
      }

      return (
        <div className="min-h-screen flex items-center justify-center bg-surface p-4">
          <div className="bg-background rounded-lg shadow-lg p-8 max-w-md w-full text-center">
            <div className="mb-4">
              <Text size="6" weight="bold" className="text-error-strong">
                Something went wrong
              </Text>
            </div>
            <div className="mb-6">
              <Text size="3" color="gray">
                {this.state.error?.message || 'An unexpected error occurred'}
              </Text>
            </div>
            <Button onClick={this.handleReset} size="3">
              Try Again
            </Button>
          </div>
        </div>
      )
    }

    return this.props.children
  }
}
