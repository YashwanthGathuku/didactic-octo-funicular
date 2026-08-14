/**
 * DaVinci's Codex Sandbox — UI Controller
 * Builds all DOM elements, manages machine selection, parameter sliders,
 * playback controls, overlay toggles, telemetry gauges, and diagnostic toasts.
 */
import { PRESETS } from '../machines/MachinePresets.js';

export class CodexUI {
  constructor({ physicsWorld, renderer, soundEngine, failureAnalyzer,
                presets, onLoadPreset, onParamChange, onPlay, onPause,
                onStep, onReset, onSpeedChange }) {
    this.physicsWorld = physicsWorld;
    this.renderer = renderer;
    this.soundEngine = soundEngine;
    this.failureAnalyzer = failureAnalyzer;
    this.presets = presets || PRESETS;

    // Callbacks from main.js
    this.onLoadPreset = onLoadPreset;
    this.onParamChange = onParamChange;
    this.onPlay = onPlay;
    this.onPause = onPause;
    this.onStep = onStep;
    this.onReset = onReset;
    this.onSpeedChange = onSpeedChange;

    this.currentPresetName = null;
    this.isPlaying = false;
    this.speed = 1.0;
    this._paramValues = {};
  }

  init() {
    this._buildSidebar();
    this._buildOverlayToggles();
    this._buildTelemetryPanel();
  }

  /* ── Public API ─────────────────────────────────────── */

  loadPreset(name) {
    this.currentPresetName = name;
    document.querySelectorAll('.machine-card').forEach(card => {
      card.classList.toggle('active', card.dataset.preset === name);
    });
    if (this.onLoadPreset) this.onLoadPreset(name);
    this.isPlaying = true;
    this._updatePlayBtn();
  }

  rebuildParams(preset) {
    if (!preset) return;
    const container = document.getElementById('params-container');
    if (!container) return;
    container.innerHTML = '';
    this._paramValues = {};

    preset.params.forEach(p => {
      this._paramValues[p.id] = p.default;

      const group = document.createElement('div');
      group.className = 'param-group';

      const labelRow = document.createElement('div');
      labelRow.className = 'param-label';

      const nameSpan = document.createElement('span');
      nameSpan.textContent = p.label;

      const valueSpan = document.createElement('span');
      valueSpan.className = 'param-value';
      valueSpan.id = `param-val-${p.id}`;
      valueSpan.textContent = `${p.default} ${p.unit}`;

      labelRow.appendChild(nameSpan);
      labelRow.appendChild(valueSpan);

      const input = document.createElement('input');
      input.type = 'range';
      input.min = p.min;
      input.max = p.max;
      input.step = p.step;
      input.value = p.default;
      input.id = `param-${p.id}`;

      input.addEventListener('input', () => {
        const val = parseFloat(input.value);
        valueSpan.textContent = `${val} ${p.unit}`;
        this._paramValues[p.id] = val;
        if (this.onParamChange) this.onParamChange(p.id, val);
      });

      group.appendChild(labelRow);
      group.appendChild(input);
      container.appendChild(group);
    });
  }

  getParamValues() {
    return { ...this._paramValues };
  }

  updateTelemetry(metrics) {
    if (!metrics) return;
    const panel = document.getElementById('telemetry-gauges');
    if (!panel) return;

    // Rebuild gauge elements dynamically based on current metrics
    const keys = Object.keys(metrics);
    keys.forEach(key => {
      let gauge = document.getElementById(`gauge-${key}`);
      if (!gauge) {
        gauge = document.createElement('div');
        gauge.className = 'telemetry-gauge';
        gauge.id = `gauge-${key}`;

        const label = document.createElement('div');
        label.className = 'telemetry-label';
        label.textContent = key.replace(/([A-Z])/g, ' $1').toUpperCase();

        const value = document.createElement('div');
        value.className = 'telemetry-value';
        value.id = `metric-${key}`;

        gauge.appendChild(label);
        gauge.appendChild(value);
        panel.appendChild(gauge);
      }

      const valEl = document.getElementById(`metric-${key}`);
      if (valEl) valEl.textContent = metrics[key];
    });
  }

  showDiagnostic(failure) {
    const toast = document.getElementById('diagnostics-toast');
    if (!toast) return;

    toast.style.display = 'flex';

    const card = document.createElement('div');
    card.className = 'diagnostic-card';

    const icon = document.createElement('div');
    icon.className = 'diag-icon';
    icon.textContent = failure.severity === 'critical' ? '🔴' : '🟡';

    const title = document.createElement('div');
    title.className = 'diag-title';
    title.textContent = failure.type === 'opposing_motion' ? '⚠️ Design Flaw Detected'
      : failure.type === 'gear_jam' ? '⚙️ Gear Jam'
      : failure.type === 'torque_exceeded' ? '💥 Torque Exceeded'
      : failure.type === 'force_exceeded' ? '💥 Force Exceeded'
      : '⚡ Impact Detected';

    const msg = document.createElement('div');
    msg.className = 'diag-message';
    msg.textContent = failure.message;

    const suggestion = document.createElement('div');
    suggestion.className = 'diag-suggestion';
    suggestion.textContent = `💡 ${failure.suggestion}`;

    card.appendChild(icon);
    card.appendChild(title);
    card.appendChild(msg);
    card.appendChild(suggestion);
    toast.appendChild(card);

    // Auto-dismiss after 8 seconds
    setTimeout(() => {
      card.style.animation = 'slideUpIn 0.4s ease reverse forwards';
      setTimeout(() => {
        card.remove();
        if (toast.children.length === 0) toast.style.display = 'none';
      }, 400);
    }, 8000);

    // Also log to diagnostics history in telemetry panel
    const historyLog = document.getElementById('diag-history');
    if (historyLog) {
      const entry = document.createElement('div');
      entry.className = 'diag-history-entry';
      entry.textContent = `[${failure.timestamp?.toFixed(1) || '0'}s] ${failure.message}`;
      historyLog.prepend(entry);
    }
  }

  /* ── Private: Build Sidebar ─────────────────────────── */

  _buildSidebar() {
    const sidebar = document.getElementById('sidebar');
    if (!sidebar) return;

    // ── Title Section ──
    const titleSection = this._createSection('⚙ Codex Inventions');

    // ── Machine Cards ──
    const presetKeys = Object.keys(this.presets);
    presetKeys.forEach(key => {
      const preset = this.presets[key];
      const card = document.createElement('div');
      card.className = 'machine-card';
      card.dataset.preset = key;

      const icon = document.createElement('div');
      icon.className = 'machine-icon';
      icon.textContent = key === 'aerialScrew' ? '🔩'
        : key === 'ornithopter' ? '🦅'
        : key === 'armoredTank' ? '⚔️'
        : key === 'springCatapult' ? '🏹'
        : '🌉';

      const textWrap = document.createElement('div');
      textWrap.className = 'machine-text';

      const name = document.createElement('div');
      name.className = 'machine-name';
      name.textContent = preset.name;

      const desc = document.createElement('div');
      desc.className = 'machine-desc';
      desc.textContent = preset.description.substring(0, 80) + '…';

      const infoBtn = document.createElement('button');
      infoBtn.className = 'machine-info-btn';
      infoBtn.textContent = 'ℹ';
      infoBtn.title = 'Historical info';
      infoBtn.onclick = (e) => {
        e.stopPropagation();
        this._showInfoModal(preset);
      };

      textWrap.appendChild(name);
      textWrap.appendChild(desc);

      card.appendChild(icon);
      card.appendChild(textWrap);
      card.appendChild(infoBtn);

      card.addEventListener('click', () => this.loadPreset(key));
      titleSection.appendChild(card);
    });

    sidebar.appendChild(titleSection);

    // ── Divider ──
    const divider = document.createElement('div');
    divider.className = 'section-divider';
    sidebar.appendChild(divider);

    // ── Parameters Section ──
    const paramsSection = this._createSection('🎛 Parameters');
    const paramsContainer = document.createElement('div');
    paramsContainer.id = 'params-container';
    paramsSection.appendChild(paramsContainer);
    sidebar.appendChild(paramsSection);

    // ── Divider ──
    sidebar.appendChild(divider.cloneNode());

    // ── Playback Controls ──
    const playbackSection = this._createSection('▶ Playback');
    const playbackBar = document.createElement('div');
    playbackBar.className = 'playback-bar';

    // Play/Pause
    this._playBtn = document.createElement('button');
    this._playBtn.className = 'playback-btn active';
    this._playBtn.textContent = '⏸';
    this._playBtn.onclick = () => {
      this.isPlaying = !this.isPlaying;
      this._updatePlayBtn();
      if (this.isPlaying) { if (this.onPlay) this.onPlay(); }
      else { if (this.onPause) this.onPause(); }
    };

    // Step
    const stepBtn = document.createElement('button');
    stepBtn.className = 'playback-btn';
    stepBtn.textContent = '⏭';
    stepBtn.title = 'Step forward';
    stepBtn.onclick = () => { if (this.onStep) this.onStep(); };

    // Reset
    const resetBtn = document.createElement('button');
    resetBtn.className = 'playback-btn';
    resetBtn.textContent = '↺';
    resetBtn.title = 'Reset';
    resetBtn.onclick = () => { if (this.onReset) this.onReset(); };

    playbackBar.appendChild(this._playBtn);
    playbackBar.appendChild(stepBtn);
    playbackBar.appendChild(resetBtn);
    playbackSection.appendChild(playbackBar);

    // Speed slider
    const speedGroup = document.createElement('div');
    speedGroup.className = 'param-group';

    const speedLabel = document.createElement('div');
    speedLabel.className = 'param-label';

    const speedName = document.createElement('span');
    speedName.textContent = 'Speed';
    const speedVal = document.createElement('span');
    speedVal.className = 'param-value speed-display';
    speedVal.textContent = '1.0x';

    speedLabel.appendChild(speedName);
    speedLabel.appendChild(speedVal);

    const speedInput = document.createElement('input');
    speedInput.type = 'range';
    speedInput.min = '0.1';
    speedInput.max = '2.0';
    speedInput.step = '0.1';
    speedInput.value = '1.0';
    speedInput.addEventListener('input', () => {
      this.speed = parseFloat(speedInput.value);
      speedVal.textContent = `${this.speed.toFixed(1)}x`;
      if (this.onSpeedChange) this.onSpeedChange(this.speed);
    });

    speedGroup.appendChild(speedLabel);
    speedGroup.appendChild(speedInput);
    playbackSection.appendChild(speedGroup);

    sidebar.appendChild(playbackSection);

    // ── Sound Controls ──
    const soundSection = this._createSection('🔊 Audio');
    const muteBtn = document.createElement('button');
    muteBtn.className = 'playback-btn';
    muteBtn.textContent = '🔊';
    muteBtn.title = 'Toggle audio';
    muteBtn.onclick = () => {
      if (this.soundEngine.muted) {
        this.soundEngine.unmute();
        muteBtn.textContent = '🔊';
      } else {
        this.soundEngine.mute();
        muteBtn.textContent = '🔇';
      }
    };
    soundSection.appendChild(muteBtn);
    sidebar.appendChild(soundSection);
  }

  /* ── Private: Build Overlay Toggles ─────────────────── */

  _buildOverlayToggles() {
    const container = document.getElementById('overlay-controls');
    if (!container) return;
    container.className = 'overlay-toggles';

    const overlays = [
      { type: 'forces', label: '⟶ Forces', title: 'Show force vectors' },
      { type: 'stress', label: '🔥 Stress', title: 'Show stress heatmap' },
      { type: 'velocity', label: '→ Velocity', title: 'Show velocity vectors' },
      { type: 'trajectory', label: '⋯ Trail', title: 'Show projectile trails' },
      { type: 'fbd', label: '📐 FBD', title: 'Free-body diagram mode' }
    ];

    overlays.forEach(({ type, label, title }) => {
      const btn = document.createElement('button');
      btn.className = 'overlay-btn';
      btn.textContent = label;
      btn.title = title;
      btn.onclick = () => {
        btn.classList.toggle('active');
        this.renderer.toggleOverlay(type);
      };
      container.appendChild(btn);
    });
  }

  /* ── Private: Build Telemetry Panel ─────────────────── */

  _buildTelemetryPanel() {
    const panel = document.getElementById('telemetry-panel');
    if (!panel) return;

    // Title & Gauges
    const gaugesHeader = document.createElement('h3');
    gaugesHeader.textContent = '📊 Telemetry';
    gaugesHeader.className = 'panel-header';
    panel.appendChild(gaugesHeader);

    const gaugesContainer = document.createElement('div');
    gaugesContainer.id = 'telemetry-gauges';
    gaugesContainer.className = 'gauges-container';
    panel.appendChild(gaugesContainer);

    // Divider
    const divider = document.createElement('div');
    divider.className = 'section-divider';
    panel.appendChild(divider);

    // Diagnostics History
    const diagHeader = document.createElement('h3');
    diagHeader.textContent = '📋 Diagnostics Log';
    diagHeader.className = 'panel-header';
    panel.appendChild(diagHeader);

    const historyLog = document.createElement('div');
    historyLog.id = 'diag-history';
    historyLog.className = 'diag-history';
    panel.appendChild(historyLog);
  }

  /* ── Private: Info Modal ────────────────────────────── */

  _showInfoModal(preset) {
    const overlay = document.getElementById('modal-overlay');
    if (!overlay) return;

    overlay.style.display = 'flex';
    overlay.innerHTML = '';

    const modal = document.createElement('div');
    modal.className = 'modal-content';

    const closeBtn = document.createElement('button');
    closeBtn.className = 'modal-close';
    closeBtn.textContent = '✕';
    closeBtn.onclick = () => { overlay.style.display = 'none'; };

    const titleEl = document.createElement('h2');
    titleEl.className = 'modal-title';
    titleEl.textContent = preset.name;

    const source = document.createElement('p');
    source.className = 'mirror-text decoded';
    source.textContent = `📜 ${preset.codexSource}`;

    const note = document.createElement('p');
    note.className = 'modal-body';
    note.textContent = preset.historicalNote;

    const desc = document.createElement('p');
    desc.style.fontStyle = 'italic';
    desc.style.opacity = '0.8';
    desc.textContent = preset.description;

    modal.appendChild(closeBtn);
    modal.appendChild(titleEl);
    modal.appendChild(source);
    modal.appendChild(desc);
    modal.appendChild(note);
    overlay.appendChild(modal);

    // Click outside to close
    overlay.onclick = (e) => {
      if (e.target === overlay) overlay.style.display = 'none';
    };
  }

  /* ── Helpers ────────────────────────────────────────── */

  _createSection(title) {
    const section = document.createElement('div');
    section.className = 'sidebar-section';

    const h3 = document.createElement('h3');
    h3.textContent = title;
    section.appendChild(h3);

    return section;
  }

  _updatePlayBtn() {
    if (!this._playBtn) return;
    this._playBtn.textContent = this.isPlaying ? '⏸' : '▶';
    this._playBtn.classList.toggle('active', this.isPlaying);
  }
}
