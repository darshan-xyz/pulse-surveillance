import React, { Component, ErrorInfo, ReactNode } from 'react';

interface Props {
  children: ReactNode;
  fallback?: ReactNode;
}

interface State {
  hasError: boolean;
  error?: Error;
}

class ErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props);
    this.state = { hasError: false };
  }

  static getDerivedStateFromError(error: Error): State {
    // Don't set error state for HMR-related errors or context errors during recovery
    const isHMRError = 
      error.message?.includes('removeChild') ||
      error.message?.includes('Fast Refresh') ||
      error.name === 'DOMException';
    
    // Also ignore context errors during error recovery (these happen when ErrorBoundary resets)
    const isContextError = 
      error.message?.includes('must be used within') ||
      error.message?.includes('UserProfileProvider');
    
    if ((isHMRError || isContextError) && import.meta.env.DEV) {
      // Return no error state for HMR/context errors - let React handle recovery
      return { hasError: false };
    }
    
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo) {
    // Ignore HMR-related errors and context errors during recovery
    const isHMRError = 
      error.message?.includes('removeChild') ||
      error.message?.includes('Fast Refresh') ||
      error.name === 'DOMException' ||
      errorInfo.componentStack?.includes('react-refresh');
    
    const isContextError = 
      error.message?.includes('must be used within') ||
      error.message?.includes('UserProfileProvider');
    
    if ((isHMRError || isContextError) && import.meta.env.DEV) {
      // Silently ignore HMR/context errors in development
      // These are expected during hot reload and error recovery
      return;
    }
    
    console.error('ErrorBoundary caught an error:', error, errorInfo);
  }

  componentDidUpdate(prevProps: Props) {
    // Reset error state when props change (useful for hot reload)
    // But be careful - only reset if it's a real prop change, not during error recovery
    if (prevProps.children !== this.props.children && !this.state.hasError) {
      this.setState({ hasError: false, error: undefined });
    }
  }

  render() {
    if (this.state.hasError) {
      // Don't show error UI for context errors - let the app recover naturally
      const isContextError = 
        this.state.error?.message?.includes('must be used within') ||
        this.state.error?.message?.includes('UserProfileProvider');
      
      if (isContextError && import.meta.env.DEV) {
        // For context errors, try to render children anyway (context might be available now)
        // This helps with HMR recovery
        return this.props.children;
      }
      
      if (this.props.fallback) {
        return this.props.fallback;
      }
      
      return (
        <div className="p-4 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg">
          <h2 className="text-lg font-semibold text-red-800 dark:text-red-200 mb-2">
            Something went wrong
          </h2>
          <p className="text-red-600 dark:text-red-400 mb-3">
            {this.state.error?.message || 'An unexpected error occurred'}
          </p>
          <button
            onClick={() => {
              // Use window.location.reload() instead of state reset for more reliable recovery
              window.location.reload();
            }}
            className="px-4 py-2 bg-red-600 text-white rounded hover:bg-red-700 transition-colors"
          >
            Reload Page
          </button>
        </div>
      );
    }

    return this.props.children;
  }
}

export default ErrorBoundary;

