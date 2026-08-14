export class FailureAnalyzer {
  constructor(physicsWorld) {
    this.physicsWorld = physicsWorld;
    this.thresholds = {
      maxTorque: 5000,
      maxForce: 10000,
      maxVelocity: 50,
      maxStress: 1.0
    };
    this.history = [];
    this.failureCallbacks = [];
    this.warningCallbacks = [];
    
    // Analysis is driven by the main game loop (main.js calls analyze() each frame)
    this._lastFailureTime = -10; // throttle: don't spam failures
  }

  setThresholds(config) {
    this.thresholds = { ...this.thresholds, ...config };
  }

  analyze() {
    const failures = [];
    const dt = 1 / 60;
    
    // 1. Check Joints (Torque, Force, Jams)
    const joints = this.physicsWorld._joints || [];
    joints.forEach(jInfo => {
      const { joint, label, type } = jInfo;
      const forceVec = joint.getReactionForce ? joint.getReactionForce(dt) : null;
      const forceMag = forceVec ? forceVec.length() : 0;
      const torque = joint.getReactionTorque ? Math.abs(joint.getReactionTorque(dt)) : 0;

      // Force Check
      if (forceMag > this.thresholds.maxForce) {
        failures.push({
          type: 'force_exceeded',
          severity: 'critical',
          component: label,
          currentValue: forceMag,
          threshold: this.thresholds.maxForce,
          message: `Component '${label}' force (${Math.round(forceMag)} N) exceeded structural limit (${this.thresholds.maxForce} N). Reinforce with additional support.`,
          suggestion: 'Reinforce with additional support or reduce payload.',
          timestamp: this.physicsWorld.time
        });
      }

      // Torque Check (mostly for revolute/gear)
      if (torque > this.thresholds.maxTorque) {
        failures.push({
          type: 'torque_exceeded',
          severity: 'critical',
          component: label,
          currentValue: torque,
          threshold: this.thresholds.maxTorque,
          message: `Joint '${label}' torque (${Math.round(torque)} N·m) exceeded limit (${this.thresholds.maxTorque} N·m).`,
          suggestion: 'Try increasing gear ratio to reduce load or reduce motor speed.',
          timestamp: this.physicsWorld.time
        });
      }

      // Gear Jam Detection
      if (type === 'revolute' && joint.isMotorEnabled && joint.isMotorEnabled()) {
        const motorSpeed = joint.getMotorSpeed();
        const currentSpeed = joint.getJointSpeed();
        const maxTorque = joint.getMaxMotorTorque();
        const currentTorque = Math.abs(joint.getMotorTorque(dt));
        
        // If we are demanding max torque but moving very slowly relative to target speed
        if (motorSpeed !== 0 && Math.abs(currentSpeed) < 0.1 && currentTorque >= maxTorque * 0.95) {
           failures.push({
             type: 'gear_jam',
             severity: 'warning',
             component: label,
             currentValue: currentSpeed,
             threshold: motorSpeed,
             message: `Gear jam at '${label}': Motor at maximum torque but mechanism is stalled.`,
             suggestion: 'Reduce load or increase motor power.',
             timestamp: this.physicsWorld.time
           });
        }
      }
    });

    // 2. Check Bodies (Velocity spikes, specific logic like opposing motion)
    const bodies = this.physicsWorld._bodies || [];
    let frontWheelDir = 0;
    let rearWheelDir = 0;

    bodies.forEach(bInfo => {
      const { body, label } = bInfo;
      const velocity = body.getLinearVelocity();
      const speed = velocity.length();

      // Velocity Spike
      if (speed > this.thresholds.maxVelocity) {
        failures.push({
          type: 'velocity_spike',
          severity: 'warning',
          component: label,
          currentValue: speed,
          threshold: this.thresholds.maxVelocity,
          message: `Impact detected at '${label}': Velocity spike of ${Math.round(speed)} m/s suggests structural failure.`,
          suggestion: 'Check for high-speed collisions or unstable spring physics.',
          timestamp: this.physicsWorld.time
        });
      }

      // Specific check for Armored Tank flaw
      if (label === 'front_wheel' && Math.abs(body.getAngularVelocity()) > 0.1) {
        frontWheelDir = Math.sign(body.getAngularVelocity());
      }
      if (label === 'rear_wheel' && Math.abs(body.getAngularVelocity()) > 0.1) {
        rearWheelDir = Math.sign(body.getAngularVelocity());
      }
    });

    // Opposing motion check
    if (frontWheelDir !== 0 && rearWheelDir !== 0 && frontWheelDir !== rearWheelDir) {
      // Prevent spamming
      const alreadyReported = this.history.find(h => h.type === 'opposing_motion' && this.physicsWorld.time - h.timestamp < 2);
      if (!alreadyReported) {
        failures.push({
          type: 'opposing_motion',
          severity: 'critical',
          component: 'Wheels',
          currentValue: 0,
          threshold: 0,
          message: `⚠️ Design Flaw Detected: Front and rear wheels are rotating in opposite directions! The gear linkage causes counter-rotation.`,
          suggestion: 'Fix: Reverse the gear ratio or add an intermediate idler gear.',
          timestamp: this.physicsWorld.time
        });
      }
    }

    // Process failures
    if (failures.length > 0) {
      this.history.push(...failures);
      this.failureCallbacks.forEach(cb => cb(failures));
    }

    // Warnings (Stress approaching threshold)
    const warnings = [];
    const stressData = this.getStressData();
    stressData.forEach((data, component) => {
      if (data.stressRatio > 0.8) {
        warnings.push({ component, data });
      }
    });

    if (warnings.length > 0) {
      this.warningCallbacks.forEach(cb => cb(warnings));
    }

    return failures;
  }

  getStressData() {
    const stressMap = new Map();
    const dt = 1 / 60;
    
    // Only looking at joints for stress for simplicity
    (this.physicsWorld._joints || []).forEach(jInfo => {
      const { joint, label } = jInfo;
      let stressRatio = 0;
      let type = 'tension';

      const forceVec = joint.getReactionForce ? joint.getReactionForce(dt) : null;
      if (forceVec) {
        const forceMag = forceVec.length();
        stressRatio = forceMag / this.thresholds.maxForce;
        
        // Very basic determination of tension vs compression based on force direction vs anchor
        // Real analysis would project force onto the axis connecting the bodies
        if (forceVec.y < 0) {
          type = 'compression';
        }
      }

      const torque = joint.getReactionTorque ? Math.abs(joint.getReactionTorque(dt)) : 0;
      const torqueRatio = torque / this.thresholds.maxTorque;
      
      if (torqueRatio > stressRatio) {
        stressRatio = torqueRatio;
        type = 'shear';
      }

      stressMap.set(joint, { stressRatio: Math.min(stressRatio, 1.0), type, label });
    });

    return stressMap;
  }

  onFailure(callback) {
    this.failureCallbacks.push(callback);
  }

  onWarning(callback) {
    this.warningCallbacks.push(callback);
  }

  getFailureHistory() {
    return this.history;
  }

  clearHistory() {
    this.history = [];
  }
}
