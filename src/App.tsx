/**
 * Main Application Shell
 * 
 * Clean light fintech theme with top navigation header and primary operations console.
 */

import React, { useState } from 'react';
import { Header } from './components/Header';
import { UploadModal } from './components/UploadModal';
import { ConnectorWizardModal } from './components/ConnectorWizardModal';
import { SavedConnectionsPanel } from './components/SavedConnectionsPanel';
import { OperationsConsole } from './components/ops/OperationsConsole';
import { SessionProvider } from './state/SessionContext';
import { Database, Plus } from 'lucide-react';

export const App: React.FC = () => {
  const [showUpload, setShowUpload] = useState(false);
  const [showConnections, setShowConnections] = useState(false);
  const [showWizard, setShowWizard] = useState(false);

  return (
    <SessionProvider>
      <div className="flex min-h-screen flex-col bg-[#F8FAFC] text-slate-900 antialiased selection:bg-indigo-100 selection:text-indigo-900">
        <Header
          onOpenUpload={() => setShowUpload(true)}
          onOpenConnectors={() => setShowConnections((v) => !v)}
        />

        <main className="flex-1 space-y-6 px-6 py-6 sm:px-8">
          {showConnections && (
            <div className="mx-auto max-w-7xl">
              <section
                aria-label="Saved source connections"
                className="fintech-card overflow-hidden"
              >
                <div className="flex items-center justify-between border-b border-slate-200 bg-slate-50 px-5 py-3.5">
                  <div className="flex items-center gap-2">
                    <Database className="h-4 w-4 text-indigo-600" />
                    <h2 className="text-xs font-bold uppercase tracking-wider text-slate-700">
                      Managed Source Connections
                    </h2>
                  </div>
                  <button
                    type="button"
                    onClick={() => setShowWizard(true)}
                    className="inline-flex items-center gap-1.5 rounded-lg border border-indigo-200 bg-indigo-50 px-3 py-1.5 text-xs font-semibold text-indigo-700 hover:bg-indigo-100"
                  >
                    <Plus className="h-3.5 w-3.5" />
                    <span>Add Connection</span>
                  </button>
                </div>
                <SavedConnectionsPanel />
              </section>
            </div>
          )}

          <OperationsConsole />
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
