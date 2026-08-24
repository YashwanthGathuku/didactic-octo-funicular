/**
 * Main application shell.
 *
 * SentinelFlow is an operational control plane, not a marketing dashboard. The
 * shell keeps global actions compact and gives the governed operations console
 * most of the viewport.
 */

import React, { useState } from 'react';
import { Database, Plus } from 'lucide-react';
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
      <div className="flex min-h-[100dvh] flex-col bg-[#F3F5F7] text-slate-950 antialiased selection:bg-slate-200 selection:text-slate-950">
        <Header
          onOpenUpload={() => setShowUpload(true)}
          onOpenConnectors={() => setShowConnections((value) => !value)}
        />

        <main className="flex-1 px-4 py-4 sm:px-6 lg:px-8 lg:py-6">
          {showConnections && (
            <div className="mx-auto mb-5 max-w-[1440px]">
              <section aria-label="Saved source connections" className="surface-panel overflow-hidden">
                <div className="flex flex-wrap items-center justify-between gap-3 border-b border-slate-200 px-4 py-3 sm:px-5">
                  <div className="flex items-center gap-2.5">
                    <Database className="h-4 w-4 text-slate-500" aria-hidden />
                    <div>
                      <h2 className="text-sm font-semibold text-slate-950">Managed source connections</h2>
                      <p className="mt-0.5 text-xs text-slate-500">Connection inventory is operational configuration, not model context.</p>
                    </div>
                  </div>
                  <button
                    type="button"
                    onClick={() => setShowWizard(true)}
                    className="secondary-action"
                  >
                    <Plus className="h-3.5 w-3.5" aria-hidden />
                    <span>Add connection</span>
                  </button>
                </div>
                <SavedConnectionsPanel />
              </section>
            </div>
          )}

          <OperationsConsole onOpenUpload={() => setShowUpload(true)} />
        </main>

        {showWizard && <ConnectorWizardModal onClose={() => setShowWizard(false)} />}
        {showUpload && (
          <UploadModal
            onClose={() => setShowUpload(false)}
            onFileIngested={() => setShowUpload(false)}
          />
        )}
      </div>
    </SessionProvider>
  );
};
