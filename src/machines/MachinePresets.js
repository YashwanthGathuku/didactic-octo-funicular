import * as planck from 'planck-js';
import { PIXELS_PER_METER } from '../physics/PhysicsWorld.js';

export const PRESETS = {
  aerialScrew: {
    name: 'The Aerial Screw',
    description: 'Leonardo\'s helical flying machine — proven by Johns Hopkins (2025) to be 42% more efficient than modern rotors',
    codexSource: 'Manuscript B, folio 83v (c. 1489)',
    historicalNote: 'While the aerodynamic geometry is validated, human power (~0.15 hp) cannot generate sufficient lift for the heavy timber/canvas structure.',
    params: [
      { id: 'rpm', label: 'Rotor RPM', min: 0, max: 300, default: 60, step: 5, unit: 'RPM' },
      { id: 'pitch', label: 'Blade Pitch', min: 10, max: 80, default: 45, step: 1, unit: '°' },
      { id: 'airDensity', label: 'Air Density', min: 0.5, max: 1.5, default: 1.225, step: 0.025, unit: 'kg/m³' }
    ],
    build: (world, params) => {
      const bodies = [];
      const joints = [];

      const base = world.createBody({
        type: 'static',
        x: 0, y: 0,
        shape: { type: 'box', width: 4, height: 0.5 },
        label: 'base',
        userData: { type: 'frame', color: '#5c4033', width: 4, height: 0.5 }
      });
      bodies.push(base);

      const shaft = world.createBody({
        type: 'dynamic',
        x: 0, y: 2,
        shape: { type: 'box', width: 0.2, height: 4 },
        density: 2.0,
        label: 'shaft',
        userData: { type: 'shaft', color: '#d4c4a1', width: 0.2, height: 4 }
      });
      bodies.push(shaft);

      const rotor = world.createBody({
        type: 'dynamic',
        x: 0, y: 3,
        shape: { type: 'box', width: 6, height: 0.1 },
        density: 0.5,
        label: 'rotor',
        userData: { type: 'rotor', color: '#f4e4c1', width: 6, height: 0.1 }
      });
      bodies.push(rotor);

      const shaftJoint = world.createRevoluteJoint({
        bodyA: base, bodyB: shaft,
        anchorX: 0, anchorY: 0,
        enableMotor: false,
        label: 'shaft_base_joint'
      });
      joints.push(shaftJoint);

      const rpm = params.find(p => p.id === 'rpm')?.value || 60;
      const motorSpeed = (rpm * 2 * Math.PI) / 60; // rad/s

      const rotorJoint = world.createRevoluteJoint({
        bodyA: shaft, bodyB: rotor,
        anchorX: 0, anchorY: 3,
        enableMotor: true,
        motorSpeed: motorSpeed,
        maxMotorTorque: 5000,
        label: 'rotor_joint'
      });
      joints.push(rotorJoint);

      return { bodies, joints, metadata: { rotorBody: rotor, pitch: params.find(p => p.id === 'pitch')?.value || 45, density: params.find(p => p.id === 'airDensity')?.value || 1.225 } };
    },
    getMetrics: (world, bodies, joints, metadata) => {
      if (!metadata.rotorBody) return {};
      const rotor = metadata.rotorBody;
      const v = rotor.getLinearVelocity().length();
      const angularV = Math.abs(rotor.getAngularVelocity());
      // Simple aerodynamic lift approximation
      const lift = 0.5 * metadata.density * Math.pow(angularV, 2) * (metadata.pitch / 45) * 5;
      
      world.applyAerodynamicForce(rotor, {
        liftCoeff: (metadata.pitch / 45) * 0.1,
        dragCoeff: 0.05,
        area: 6,
        airDensity: metadata.density,
        windSpeed: {x:0, y:0}
      });
      
      return {
        liftForce: lift.toFixed(1),
        rpm: ((angularV * 60) / (2 * Math.PI)).toFixed(1)
      };
    }
  },

  ornithopter: {
    name: 'The Ornithopter',
    description: 'A machine designed to achieve flight by flapping its wings like a bird.',
    codexSource: 'Codex Atlanticus, folio 276r (c. 1485)',
    historicalNote: 'Da Vinci studied bird flight extensively but underestimated the power-to-weight ratio required for human flight.',
    params: [
      { id: 'frequency', label: 'Flap Frequency', min: 0.5, max: 5.0, default: 2.0, step: 0.1, unit: 'Hz' },
      { id: 'span', label: 'Wing Span Multiplier', min: 0.5, max: 2.0, default: 1.0, step: 0.1, unit: 'x' },
      { id: 'pitch', label: 'Wing Pitch', min: 0, max: 45, default: 15, step: 1, unit: '°' }
    ],
    build: (world, params) => {
      const bodies = [];
      const joints = [];
      
      const freq = params.find(p => p.id === 'frequency')?.value || 2.0;
      const spanMult = params.find(p => p.id === 'span')?.value || 1.0;
      
      const fuselage = world.createBody({
        type: 'dynamic', x: 0, y: 5, shape: { type: 'box', width: 2, height: 1 },
        label: 'fuselage', userData: { type: 'frame', color: '#5c4033', width: 2, height: 1 }
      });
      bodies.push(fuselage);
      
      const crank = world.createBody({
        type: 'dynamic', x: 0, y: 5, shape: { type: 'circle', radius: 0.4 },
        label: 'crank', userData: { type: 'gear', color: '#b5a642', radius: 0.4 }
      });
      bodies.push(crank);
      
      const crankJoint = world.createRevoluteJoint({
        bodyA: fuselage, bodyB: crank, anchorX: 0, anchorY: 5,
        enableMotor: true, motorSpeed: freq * Math.PI * 2, maxMotorTorque: 5000, label: 'crank_motor'
      });
      joints.push(crankJoint);
      
      const wingW = 4 * spanMult;
      const leftWing = world.createBody({
        type: 'dynamic', x: -1 - wingW/2, y: 5.5, shape: { type: 'box', width: wingW, height: 0.1 },
        label: 'left_wing', userData: { type: 'wing', color: '#f4e4c1', width: wingW, height: 0.1 }
      });
      bodies.push(leftWing);
      
      const rightWing = world.createBody({
        type: 'dynamic', x: 1 + wingW/2, y: 5.5, shape: { type: 'box', width: wingW, height: 0.1 },
        label: 'right_wing', userData: { type: 'wing', color: '#f4e4c1', width: wingW, height: 0.1 }
      });
      bodies.push(rightWing);
      
      const lwJoint = world.createRevoluteJoint({ bodyA: fuselage, bodyB: leftWing, anchorX: -1, anchorY: 5.5, label: 'lw_pivot' });
      const rwJoint = world.createRevoluteJoint({ bodyA: fuselage, bodyB: rightWing, anchorX: 1, anchorY: 5.5, label: 'rw_pivot' });
      joints.push(lwJoint, rwJoint);
      
      const lwRod = world.createDistanceJoint({
        bodyA: crank, bodyB: leftWing, anchorAX: 0, anchorAY: 5.4, anchorBX: -2, anchorBY: 5.5, label: 'lw_rod'
      });
      const rwRod = world.createDistanceJoint({
        bodyA: crank, bodyB: rightWing, anchorAX: 0, anchorAY: 4.6, anchorBX: 2, anchorBY: 5.5, label: 'rw_rod'
      });
      joints.push(lwRod, rwRod);

      return { bodies, joints, metadata: { leftWing, rightWing, fuselage } };
    },
    getMetrics: (world, bodies, joints, metadata) => {
      const alt = metadata.fuselage.getPosition().y;
      return { altitude: alt.toFixed(2) };
    }
  },

  armoredTank: {
    name: 'The Armored Tank',
    description: 'A turtle-like armored vehicle armed with cannons.',
    codexSource: 'Codex Arundel, f. 1030 (c. 1487)',
    historicalNote: 'Da Vinci intentionally designed the gears such that the front and rear wheels would turn in opposite directions, rendering the tank immobile. This may have been to prevent the design from being used by unauthorized builders.',
    params: [
      { id: 'crankTorque', label: 'Crank Torque', min: 100, max: 10000, default: 5000, step: 100, unit: 'N·m' },
      { id: 'frontGearRatio', label: 'Front Gear Ratio', min: -5, max: 5, default: 1, step: 0.5, unit: ':1' },
      { id: 'rearGearRatio', label: 'Rear Gear Ratio', min: -5, max: 5, default: -1, step: 0.5, unit: ':1' } // THE FLAW
    ],
    build: (world, params) => {
      const bodies = [];
      const joints = [];
      
      const torque = params.find(p => p.id === 'crankTorque')?.value || 5000;
      const fRatio = params.find(p => p.id === 'frontGearRatio')?.value || 1;
      const rRatio = params.find(p => p.id === 'rearGearRatio')?.value || -1;
      
      const ground = world.createBody({
        type: 'static', x: 0, y: -2, shape: { type: 'box', width: 20, height: 1 },
        label: 'ground', userData: { type: 'frame', color: '#2d1f0f', width: 20, height: 1 }
      });
      bodies.push(ground);

      const hull = world.createBody({
        type: 'dynamic', x: 0, y: 0, shape: { type: 'box', width: 6, height: 1.5 }, density: 5.0,
        label: 'hull', userData: { type: 'hull', color: '#5c4033', width: 6, height: 1.5 }
      });
      bodies.push(hull);
      
      const crank = world.createBody({
        type: 'dynamic', x: 0, y: 0, shape: { type: 'circle', radius: 0.5 },
        label: 'crank', userData: { type: 'gear', color: '#b5a642', radius: 0.5 }
      });
      bodies.push(crank);
      
      const crankJoint = world.createRevoluteJoint({
        bodyA: hull, bodyB: crank, anchorX: 0, anchorY: 0,
        enableMotor: true, motorSpeed: -2, maxMotorTorque: torque, label: 'main_crank'
      });
      joints.push(crankJoint);
      
      // Wheels
      const fWheel = world.createBody({
        type: 'dynamic', x: 2, y: -1, shape: { type: 'circle', radius: 0.8 }, friction: 0.9,
        label: 'front_wheel', userData: { type: 'wheel', color: '#333', radius: 0.8 }
      });
      const rWheel = world.createBody({
        type: 'dynamic', x: -2, y: -1, shape: { type: 'circle', radius: 0.8 }, friction: 0.9,
        label: 'rear_wheel', userData: { type: 'wheel', color: '#333', radius: 0.8 }
      });
      bodies.push(fWheel, rWheel);
      
      const fJoint = world.createRevoluteJoint({ bodyA: hull, bodyB: fWheel, anchorX: 2, anchorY: -1, label: 'front_axis' });
      const rJoint = world.createRevoluteJoint({ bodyA: hull, bodyB: rWheel, anchorX: -2, anchorY: -1, label: 'rear_axis' });
      joints.push(fJoint, rJoint);
      
      // THE GEAR LINKAGE
      const fGear = world.createGearJoint({
        bodyA: hull, bodyB: hull, joint1: crankJoint, joint2: fJoint, ratio: fRatio, label: 'front_gear'
      });
      const rGear = world.createGearJoint({
        bodyA: hull, bodyB: hull, joint1: crankJoint, joint2: rJoint, ratio: rRatio, label: 'rear_gear'
      });
      joints.push(fGear, rGear);

      return { bodies, joints, metadata: { hull } };
    },
    getMetrics: (world, bodies, joints, metadata) => {
      const v = metadata.hull.getLinearVelocity();
      return { speed: v.length().toFixed(2) + ' m/s' };
    }
  },

  springCatapult: {
    name: 'Spring Catapult',
    description: 'A catapult powered by the tension of bent wood or springs.',
    codexSource: 'Codex Atlanticus, folio 140a (c. 1485)',
    historicalNote: 'Uses leaf springs instead of traditional torsion bundles, allowing for a more compact and precise siege weapon.',
    params: [
      { id: 'stiffness', label: 'Spring Stiffness', min: 1, max: 20, default: 10, step: 1, unit: 'Hz' },
      { id: 'projectileMass', label: 'Projectile Mass', min: 1, max: 50, default: 10, step: 1, unit: 'kg' }
    ],
    build: (world, params) => {
      const bodies = [];
      const joints = [];
      
      const stiffness = params.find(p => p.id === 'stiffness')?.value || 10;
      const mass = params.find(p => p.id === 'projectileMass')?.value || 10;
      
      const ground = world.createBody({
        type: 'static', x: 0, y: -2, shape: { type: 'box', width: 20, height: 1 },
        label: 'ground', userData: { type: 'frame', color: '#2d1f0f', width: 20, height: 1 }
      });
      bodies.push(ground);

      const base = world.createBody({
        type: 'static', x: -5, y: -1, shape: { type: 'box', width: 3, height: 1 },
        label: 'base', userData: { type: 'frame', color: '#5c4033', width: 3, height: 1 }
      });
      bodies.push(base);
      
      const arm = world.createBody({
        type: 'dynamic', x: -5, y: 1, shape: { type: 'box', width: 0.4, height: 4 },
        label: 'arm', userData: { type: 'arm', color: '#d4c4a1', width: 0.4, height: 4 }
      });
      bodies.push(arm);
      
      const armJoint = world.createRevoluteJoint({
        bodyA: base, bodyB: arm, anchorX: -5, anchorY: -0.5,
        enableLimit: true, lowerAngle: -Math.PI/2, upperAngle: Math.PI/4, label: 'arm_pivot'
      });
      joints.push(armJoint);
      
      // Initially bent back
      arm.setTransform(new planck.Vec2(-5, 1), -Math.PI/2);
      
      const spring = world.createDistanceJoint({
        bodyA: base, bodyB: arm, anchorAX: -6.5, anchorAY: -0.5, anchorBX: -5, anchorBY: 2.5,
        stiffness: stiffness, damping: 0.1, length: 1.0, label: 'leaf_spring'
      });
      joints.push(spring);

      const projectile = world.createBody({
        type: 'dynamic', x: -5 + 3, y: 3, shape: { type: 'circle', radius: 0.3 }, density: mass / (Math.PI * 0.3 * 0.3),
        label: 'projectile', userData: { type: 'projectile', color: '#6b1c23', radius: 0.3 }
      });
      bodies.push(projectile);
      
      return { bodies, joints, metadata: { arm, projectile, spring } };
    },
    getMetrics: (world, bodies, joints, metadata) => {
      const v = metadata.projectile.getLinearVelocity();
      return { speed: v.length().toFixed(1) + ' m/s' };
    }
  },

  selfSupportingBridge: {
    name: 'Self-Supporting Bridge',
    description: 'An emergency bridge design that requires no nails or ropes, held together entirely by friction and gravity.',
    codexSource: 'Codex Atlanticus, folio 69v (c. 1487)',
    historicalNote: 'Brilliant structural design exploiting interlocking beams where downward force increases the friction holding the bridge together.',
    params: [
      { id: 'loadMass', label: 'Load Mass', min: 10, max: 1000, default: 100, step: 10, unit: 'kg' },
      { id: 'friction', label: 'Wood Friction', min: 0.1, max: 1.0, default: 0.6, step: 0.1, unit: 'μ' }
    ],
    build: (world, params) => {
      const bodies = [];
      const joints = [];
      
      const friction = params.find(p => p.id === 'friction')?.value || 0.6;
      const mass = params.find(p => p.id === 'loadMass')?.value || 100;
      
      const ground = world.createBody({
        type: 'static', x: 0, y: -4, shape: { type: 'box', width: 20, height: 1 },
        label: 'ground', userData: { type: 'frame', color: '#2d1f0f', width: 20, height: 1 }
      });
      bodies.push(ground);
      
      const leftAbut = world.createBody({
        type: 'static', x: -6, y: -3, shape: { type: 'box', width: 2, height: 1 }, friction,
        label: 'abutment_l', userData: { type: 'frame', color: '#5c4033', width: 2, height: 1 }
      });
      const rightAbut = world.createBody({
        type: 'static', x: 6, y: -3, shape: { type: 'box', width: 2, height: 1 }, friction,
        label: 'abutment_r', userData: { type: 'frame', color: '#5c4033', width: 2, height: 1 }
      });
      bodies.push(leftAbut, rightAbut);
      
      // Simple interlocking representation
      const logProps = { type: 'dynamic', friction, density: 1.0 };
      const log1 = world.createBody({ ...logProps, x: -4, y: -2, shape: { type: 'box', width: 4, height: 0.3 }, label: 'log1', userData: { type: 'log', color: '#d4c4a1', width: 4, height: 0.3 } });
      log1.setAngle(Math.PI/6);
      
      const log2 = world.createBody({ ...logProps, x: 4, y: -2, shape: { type: 'box', width: 4, height: 0.3 }, label: 'log2', userData: { type: 'log', color: '#d4c4a1', width: 4, height: 0.3 } });
      log2.setAngle(-Math.PI/6);
      
      const log3 = world.createBody({ ...logProps, x: 0, y: -1, shape: { type: 'box', width: 5, height: 0.3 }, label: 'log3', userData: { type: 'log', color: '#d4c4a1', width: 5, height: 0.3 } });
      
      bodies.push(log1, log2, log3);

      const load = world.createBody({
        type: 'dynamic', x: 0, y: 1, shape: { type: 'box', width: 1, height: 1 }, density: mass, friction,
        label: 'load', userData: { type: 'load', color: '#6b1c23', width: 1, height: 1 }
      });
      bodies.push(load);

      return { bodies, joints, metadata: { load } };
    },
    getMetrics: (world, bodies, joints, metadata) => {
      const pos = metadata.load.getPosition();
      return { loadHeight: pos.y.toFixed(2) + ' m' };
    }
  }
};
