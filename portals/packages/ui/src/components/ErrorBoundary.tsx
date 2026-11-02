import { Component, type ReactNode, type ErrorInfo } from 'react'

interface Props {
  children: ReactNode
  // What to show instead of children once one throws. reset renders children again.
  fallback: (error: Error, reset: () => void) => ReactNode
  onError?: (error: Error, errorInfo: ErrorInfo) => void
}

interface State {
  error: Error | null
}

export class ErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props)
    this.state = { error: null }
  }

  static getDerivedStateFromError(error: Error): State {
    return { error }
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo): void {
    this.props.onError?.(error, errorInfo)
  }

  handleReset = (): void => {
    this.setState({ error: null })
  }

  render(): ReactNode {
    if (this.state.error) {
      return this.props.fallback(this.state.error, this.handleReset)
    }

    return this.props.children
  }
}
