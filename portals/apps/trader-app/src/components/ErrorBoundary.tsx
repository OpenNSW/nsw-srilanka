import { Component, type ReactNode, type ErrorInfo } from 'react'
import { Button, Text } from '@radix-ui/themes'
import { Translation } from 'react-i18next'
import { logger } from '../utils/logger'

interface Props {
  children: ReactNode
  fallback?: ReactNode
}

interface State {
  hasError: boolean
  error: Error | null
}

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
        <Translation>
          {(t) => (
            <div className="min-h-screen flex items-center justify-center bg-app-bg p-4">
              <div className="bg-app-surface rounded-lg shadow-lg p-8 max-w-md w-full text-center">
                <div className="mb-4">
                  <Text size="6" weight="bold" className="text-error-strong">
                    {t('common.error.title')}
                  </Text>
                </div>
                <div className="mb-6">
                  <Text size="3" color="gray">
                    {this.state.error?.message || t('common.error.unexpected')}
                  </Text>
                </div>
                <Button onClick={this.handleReset} size="3">
                  {t('common.error.tryAgain')}
                </Button>
              </div>
            </div>
          )}
        </Translation>
      )
    }

    return this.props.children
  }
}
