import React, { type ErrorInfo, type ReactNode } from "react";

interface RootErrorBoundaryProps {
  children: ReactNode;
}

interface RootErrorBoundaryState {
  error: Error | null;
}

export class RootErrorBoundary extends React.Component<
  RootErrorBoundaryProps,
  RootErrorBoundaryState
> {
  state: RootErrorBoundaryState = {
    error: null,
  };

  static getDerivedStateFromError(error: Error): RootErrorBoundaryState {
    return { error };
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo) {
    console.error("RootErrorBoundary caught an uncaught render error", error, errorInfo);
  }

  private handleReload = () => {
    window.location.reload();
  };

  private handleGoHome = () => {
    window.location.assign("/");
  };

  render() {
    if (this.state.error) {
      const message = this.state.error.message?.trim() || "Unknown error";

      return (
        <div className="flex min-h-screen items-center justify-center bg-slate-950 px-6 py-10 text-slate-50">
          <div className="w-full max-w-xl rounded-3xl border border-white/10 bg-white/5 p-8 shadow-2xl shadow-black/30 backdrop-blur">
            <div className="inline-flex rounded-full border border-rose-400/30 bg-rose-500/10 px-3 py-1 text-xs font-medium text-rose-200">
              Application Error
            </div>
            <h1 className="mt-4 text-2xl font-semibold tracking-tight">
              页面遇到未捕获错误
            </h1>
            <p className="mt-3 text-sm leading-6 text-slate-300">
              根级错误边界已经接管当前页面。你可以刷新重试，或返回首页重新进入工作流。
            </p>
            <div className="mt-6 flex flex-wrap gap-3">
              <button
                type="button"
                className="rounded-full bg-white px-4 py-2 text-sm font-medium text-slate-950 transition hover:bg-slate-200"
                onClick={this.handleReload}
              >
                刷新页面
              </button>
              <button
                type="button"
                className="rounded-full border border-white/20 px-4 py-2 text-sm font-medium text-slate-100 transition hover:bg-white/10"
                onClick={this.handleGoHome}
              >
                返回首页
              </button>
            </div>
            <details className="mt-6 rounded-2xl border border-white/10 bg-black/20 p-4 text-xs text-slate-300">
              <summary className="cursor-pointer list-none font-medium text-slate-100">
                技术详情
              </summary>
              <pre className="mt-3 overflow-x-auto whitespace-pre-wrap break-all text-[11px] leading-5 text-rose-100">
                {message}
              </pre>
            </details>
          </div>
        </div>
      );
    }

    return this.props.children;
  }
}
