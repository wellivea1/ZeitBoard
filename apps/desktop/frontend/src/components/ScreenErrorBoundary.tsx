import { Component, type ReactNode } from "react";

export class ScreenErrorBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
  state = { failed: false };

  static getDerivedStateFromError() {
    return { failed: true };
  }

  render() {
    if (!this.state.failed) return this.props.children;
    return (
      <section className="panel empty-state" role="alert" aria-labelledby="view-error-title">
        <h1 id="view-error-title">This view could not open</h1>
        <p>Your saved records have not been changed. Try again or open another view.</p>
        <div className="page-actions">
          <button className="button primary" onClick={() => this.setState({ failed: false })}>
            Try again
          </button>
          <button className="button secondary" onClick={() => window.location.reload()}>
            Reload app
          </button>
          <a className="button secondary" href="#/home">
            Go to Home
          </a>
        </div>
      </section>
    );
  }
}
