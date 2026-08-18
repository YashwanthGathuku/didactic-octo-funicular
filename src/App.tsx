/**
 * The application shell.
 *
 * Almost nothing lives here now, which is the change. What it held before was
 * the state of the whole product: a partner list, a contract list, a set of
 * expectation occurrences, an in-memory file list, an incident list, and a hash
 * chain -- none of which came from a server, and all of which rendered
 * perfectly with the gateway switched off.
 *
 * Every screen underneath reads from the gateway and renders an explicit state
 * when it cannot. There is no path through this tree that produces a value the
 * server did not send.
 */

import React, { useState } from 'react';
import { Header } from './components/Header';
import { UploadModal } from './components/UploadModal';
import { ConnectorWizardModal } from './components/ConnectorWizardModal';
import { SavedConnectionsPanel } from './components/SavedConnectionsPanel';
import { OperationsConsole } from './components/ops/OperationsConsole';
import { SessionProvider } from './state/SessionContext';

export const App: React.FC = () => {
  const [showUpload, setShowUpload] = useState(false);
  const [showConnections, setShowConnections] = useState(false);
  const [showWizard, setShowWizard] = useState(false);

  return (
    <SessionProvider>
      <div className="flex min-h-screen flex-col bg-slate-950 text-slate-200">
        <Header
          onOpenUpload={() => setShowUpload(true)}
          onOpenConnectors={() => setShowConnections((v) => !v)}
        />

        <main className="flex-1 space-y-4 p-6">
          {showConnections && (
            <section
              aria-label="Saved source connections"
              className="rounded border border-slate-800 bg-slate-900/40"
            >
              <div className="flex items-center justify-between border-b border-slate-800 px-3 py-2">
                <h2 className="text-xs font-semibold uppercase tracking-wide text-slate-400">
                  Source connections
                </h2>
                <button
                  type="button"
                  onClick={() => setShowWizard(true)}
                  className="rounded border border-sky-700 px-2 py-1 text-[11px] text-sky-300 hover:bg-sky-950/40"
                >
                  Add a connection
                </button>
              </div>
              <SavedConnectionsPanel />
            </section>
          )}

          <OperationsConsole />
        </main>

        {showWizard && <ConnectorWizardModal onClose={() => setShowWizard(false)} />}
        {showUpload && (
          <UploadModal
            onClose={() => setShowUpload(false)}
            // The ingest response is not held in local state and rendered.
            // It used to be assembled into a FileInstance with invented
            // fields; now the artifact screen reads the artifact back from
            // the server, which is the record either way.
            onFileIngested={() => setShowUpload(false)}
          />
        )}
      </div>
    </SessionProvider>
  );
};
