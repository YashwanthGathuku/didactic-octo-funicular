import * as Tone from 'tone';

export class SoundEngine {
  constructor() {
    this.isInitialized = false;
    this.muted = false;
    this.masterVolume = new Tone.Gain(0.8).toDestination();
    this.lastPlayTime = 0;
  }

  async init() {
    if (this.isInitialized) return;
    await Tone.start();
    
    // Synths
    this.gearSynth = new Tone.FMSynth({
      harmonicity: 3,
      modulationIndex: 10,
      oscillator: { type: 'sine' },
      envelope: { attack: 0.01, decay: 0.1, sustain: 0, release: 0.1 }
    }).connect(this.masterVolume);
    
    this.impactSynth = new Tone.MembraneSynth({
      pitchDecay: 0.05,
      octaves: 2,
      oscillator: { type: 'sine' },
      envelope: { attack: 0.001, decay: 0.2, sustain: 0, release: 0.2 }
    }).connect(this.masterVolume);

    this.creakSynth = new Tone.NoiseSynth({
      noise: { type: 'pink' },
      envelope: { attack: 0.1, decay: 0.2, sustain: 0.1, release: 0.5 }
    });
    this.creakFilter = new Tone.Filter(800, 'lowpass').connect(this.masterVolume);
    this.creakSynth.connect(this.creakFilter);

    this.windSynth = new Tone.NoiseSynth({
      noise: { type: 'brown' },
      envelope: { attack: 0.5, decay: 0.1, sustain: 1, release: 1 }
    });
    this.windFilter = new Tone.Filter(400, 'bandpass').connect(this.masterVolume);
    this.windSynth.connect(this.windFilter);
    
    this.ambientNoise = new Tone.Noise('brown');
    this.ambientFilter = new Tone.Filter(200, 'lowpass').connect(this.masterVolume);
    this.ambientGain = new Tone.Gain(0.05).connect(this.ambientFilter);
    this.ambientNoise.connect(this.ambientGain);

    this.isInitialized = true;
  }

  _canPlay() {
    if (!this.isInitialized || this.muted) return false;
    const now = Tone.now();
    // Rate limit to ~10 sounds per second (0.1s)
    if (now - this.lastPlayTime < 0.1) return false;
    this.lastPlayTime = now;
    return true;
  }

  playGearClick(rpm) {
    if (!this._canPlay()) return;
    // Base frequency increases with RPM
    const freq = 200 + Math.abs(rpm);
    this.gearSynth.triggerAttackRelease(freq, '16n');
  }

  playImpact(force) {
    if (!this._canPlay()) return;
    const velocity = Math.min(1, force / 100);
    this.impactSynth.triggerAttackRelease('C2', '8n', undefined, velocity);
  }

  playCreak(stressRatio) {
    if (!this._canPlay() || stressRatio < 0.6) return;
    this.creakSynth.triggerAttackRelease('8n');
  }

  playSpringRelease(tension) {
    if (!this._canPlay()) return;
    // Pluck sound approximation
    const freq = 100 + tension * 10;
    this.gearSynth.triggerAttackRelease(freq, '8n');
  }

  playWindWhoosh(velocity) {
    if (!this.isInitialized || this.muted) return;
    // Map velocity to filter cutoff
    const cutoff = 200 + Math.abs(velocity) * 50;
    this.windFilter.frequency.rampTo(cutoff, 0.1);
    this.windSynth.triggerAttackRelease('4n');
  }

  startAmbient() {
    if (!this.isInitialized) return;
    this.ambientNoise.start();
  }

  stopAmbient() {
    if (!this.isInitialized) return;
    this.ambientNoise.stop();
  }

  setMasterVolume(level) {
    this.masterVolume.gain.rampTo(Math.max(0, Math.min(1, level)), 0.1);
  }

  mute() {
    this.muted = true;
    this.masterVolume.gain.rampTo(0, 0.1);
  }

  unmute() {
    this.muted = false;
    this.masterVolume.gain.rampTo(0.8, 0.1);
  }

  isMuted() {
    return this.muted;
  }

  dispose() {
    if (!this.isInitialized) return;
    this.gearSynth.dispose();
    this.impactSynth.dispose();
    this.creakSynth.dispose();
    this.windSynth.dispose();
    this.ambientNoise.dispose();
    this.masterVolume.dispose();
  }
}
