/**
 * DaVinci's Codex Sandbox — Main Entry Point
 * Bootstraps physics, rendering, audio, diagnostics, and UI.
 */
import { PhysicsWorld, PIXELS_PER_METER } from './physics/PhysicsWorld.js';
import { FailureAnalyzer } from './diagnostics/FailureAnalyzer.js';
import { CanvasRenderer } from './renderer/CanvasRenderer.js';
import { SoundEngine } from './audio/SoundEngine.js';
import { CodexUI } from './ui/CodexUI.js';
import { PRESETS } from './machines/MachinePresets.js';
import './style.css';

/* ── Application State ────────────────────────────────── */
let physicsWorld, failureAnalyzer, renderer, soundEngine, ui;
let currentPresetName = null;
let currentPresetResult = null; // { bodies, joints, metadata }
let playing = false;
let playbackSpeed = 1.0;

/* ── Preset Loading ───────────────────────────────────── */
function loadPreset(name) {
  const preset = PRESETS[name];
  if (!preset) return;

  // Reset physics world
  physicsWorld.reset();
  failureAnalyzer.clearHistory();

  // Build machine — map param defaults to value
  const params = preset.params.map(p => ({ ...p, value: p.default }));
  currentPresetResult = preset.build(physicsWorld, params);
  currentPresetName = name;

  // Auto-play on load
  playing = true;
}

function rebuildCurrentPreset(paramValues) {
  const preset = PRESETS[currentPresetName];
  if (!preset) return;

  physicsWorld.reset();
  failureAnalyzer.clearHistory();

  const params = preset.params.map(p => ({
    ...p,
    value: paramValues[p.id] !== undefined ? paramValues[p.id] : p.default
  }));
  currentPresetResult = preset.build(physicsWorld, params);
}

/* ── Bootstrap ────────────────────────────────────────── */
document.addEventListener('DOMContentLoaded', () => {
  const canvas = document.getElementById('physics-canvas');
  if (!canvas) {
    console.error('Canvas element #physics-canvas not found!');
    return;
  }

  // Size canvas to fill its container
  const container = canvas.parentElement;
  canvas.width = container.clientWidth;
  canvas.height = container.clientHeight;

  // ── Core Engine ──
  physicsWorld = new PhysicsWorld();
  failureAnalyzer = new FailureAnalyzer(physicsWorld);
  renderer = new CanvasRenderer(canvas, physicsWorld, failureAnalyzer);
  soundEngine = new SoundEngine();

  // ── UI ──
  ui = new CodexUI({
    physicsWorld,
    renderer,
    soundEngine,
    failureAnalyzer,
    presets: PRESETS,
    onLoadPreset: (name) => {
      loadPreset(name);
      ui.rebuildParams(PRESETS[name]);
    },
    onParamChange: (paramId, value) => {
      // Collect all current param values from UI and rebuild
      const paramValues = ui.getParamValues();
      paramValues[paramId] = value;
      rebuildCurrentPreset(paramValues);
    },
    onPlay: () => { playing = true; },
    onPause: () => { playing = false; },
    onStep: () => {
      physicsWorld.step();
      failureAnalyzer.analyze();
      renderer.render();
    },
    onReset: () => {
      if (currentPresetName) {
        loadPreset(currentPresetName);
        ui.rebuildParams(PRESETS[currentPresetName]);
      }
    },
    onSpeedChange: (speed) => { playbackSpeed = speed; }
  });
  ui.init();

  // ── Wire Failure Diagnostics → UI Toasts ──
  failureAnalyzer.onFailure((failures) => {
    failures.forEach(f => ui.showDiagnostic(f));
  });

  // ── Wire Collisions → Sound ──
  physicsWorld.onCollision(({ impactForce }) => {
    if (soundEngine.isInitialized && impactForce > 5) {
      soundEngine.playImpact(impactForce);
    }
  });

  // ── Audio Init on First Click (Web Audio Policy) ──
  let audioInitDone = false;
  canvas.addEventListener('click', async (e) => {
    if (!audioInitDone) {
      await soundEngine.init();
      soundEngine.startAmbient();
      audioInitDone = true;
    }

    // FBD body selection
    if (renderer.isOverlayActive('fbd')) {
      const rect = canvas.getBoundingClientRect();
      const body = renderer.getBodyAtPoint(e.clientX - rect.left, e.clientY - rect.top);
      renderer.selectedBody = body || null;
    }
  });

  // ── Window Resize ──
  window.addEventListener('resize', () => {
    canvas.width = container.clientWidth;
    canvas.height = container.clientHeight;
    renderer.resize();
  });

  // ── Load Default Preset ──
  loadPreset('aerialScrew');
  ui.rebuildParams(PRESETS.aerialScrew);

  // ── Main Game Loop ──
  function loop() {
    if (playing) {
      // Apply speed multiplier
      physicsWorld.setSpeed(playbackSpeed);
      physicsWorld.step();

      // Run diagnostics
      failureAnalyzer.analyze();

      // Apply per-frame aerodynamic forces for preset
      if (currentPresetName && PRESETS[currentPresetName].getMetrics && currentPresetResult) {
        const metrics = PRESETS[currentPresetName].getMetrics(
          physicsWorld,
          currentPresetResult.bodies,
          currentPresetResult.joints,
          currentPresetResult.metadata
        );
        ui.updateTelemetry(metrics);
      }
    }

    renderer.render();
    requestAnimationFrame(loop);
  }

  requestAnimationFrame(loop);
});
