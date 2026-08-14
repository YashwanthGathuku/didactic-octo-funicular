import * as planck from 'planck-js';

export const PIXELS_PER_METER = 50;

export class PhysicsWorld {
  constructor(options = {}) {
    const gravity = options.gravity || new planck.Vec2(0, -10);
    this._world = new planck.World({ gravity });
    this._time = 0;
    this._stepCount = 0;
    this._playing = false;
    this._speedMultiplier = 1.0;
    this._stepCallbacks = [];
    this._collisionCallbacks = [];
    this._bodies = [];
    this._joints = [];

    // Collision listening
    this._world.on('begin-contact', (contact) => {
      const fixtureA = contact.getFixtureA();
      const fixtureB = contact.getFixtureB();
      const bodyA = fixtureA.getBody();
      const bodyB = fixtureB.getBody();

      // Basic impact force estimation (delta V)
      const vA = bodyA.getLinearVelocity();
      const vB = bodyB.getLinearVelocity();
      const relativeVelocity = planck.Vec2.sub(vA, vB);
      const impactForce = relativeVelocity.length() * (bodyA.getMass() + bodyB.getMass()) * 0.5;

      const manifold = contact.getManifold();
      let contactPoint = new planck.Vec2(0, 0);
      if (manifold.pointCount > 0) {
        // Approximate contact point
        contactPoint = bodyA.getWorldPoint(manifold.localPoint);
      }

      this._collisionCallbacks.forEach(cb => cb({
        bodyA, bodyB, impactForce, contactPoint
      }));
    });
  }

  get world() { return this._world; }
  get time() { return this._time; }
  get stepCount() { return this._stepCount; }

  createBody(config) {
    const { 
      type = 'dynamic', 
      x = 0, y = 0, 
      angle = 0, 
      shape, 
      density = 1.0, 
      friction = 0.3, 
      restitution = 0.1, 
      label = 'body', 
      userData = {} 
    } = config;

    const body = this._world.createBody({
      type,
      position: new planck.Vec2(x, y),
      angle
    });

    let planckShape;
    if (shape.type === 'box') {
      planckShape = new planck.Box(shape.width / 2, shape.height / 2);
    } else if (shape.type === 'circle') {
      planckShape = new planck.Circle(shape.radius);
    } else if (shape.type === 'polygon') {
      planckShape = new planck.Polygon(shape.vertices.map(v => new planck.Vec2(v.x, v.y)));
    } else {
      planckShape = new planck.Box(0.5, 0.5); // Fallback
    }

    body.createFixture({
      shape: planckShape,
      density,
      friction,
      restitution
    });

    const bodyData = { body, label, userData };
    this._bodies.push(bodyData);
    body.setUserData(bodyData); // Circular reference for easy access

    return body;
  }

  createRevoluteJoint(config) {
    const {
      bodyA, bodyB,
      anchorX, anchorY,
      motorSpeed = 0,
      maxMotorTorque = 100,
      enableMotor = false,
      enableLimit = false,
      lowerAngle = 0,
      upperAngle = 0,
      label = 'revolute'
    } = config;

    const jointDef = {
      motorSpeed,
      maxMotorTorque,
      enableMotor,
      enableLimit,
      lowerAngle,
      upperAngle
    };

    const joint = this._world.createJoint(new planck.RevoluteJoint(
      jointDef, 
      bodyA, 
      bodyB, 
      new planck.Vec2(anchorX, anchorY)
    ));
    
    this._joints.push({ joint, type: 'revolute', label, bodyA, bodyB });
    joint.setUserData({ label, type: 'revolute' });
    return joint;
  }

  createGearJoint(config) {
    const { bodyA, bodyB, joint1, joint2, ratio = 1.0, label = 'gear' } = config;
    
    const joint = this._world.createJoint(new planck.GearJoint({
      ratio
    }, bodyA, bodyB, joint1, joint2));

    this._joints.push({ joint, type: 'gear', label, bodyA, bodyB });
    joint.setUserData({ label, type: 'gear' });
    return joint;
  }

  createPrismaticJoint(config) {
    const {
      bodyA, bodyB,
      anchorX, anchorY,
      axisX, axisY,
      enableMotor = false,
      motorSpeed = 0,
      maxMotorForce = 100,
      enableLimit = false,
      lowerTranslation = 0,
      upperTranslation = 0,
      label = 'prismatic'
    } = config;

    const jointDef = {
      enableMotor,
      motorSpeed,
      maxMotorForce,
      enableLimit,
      lowerTranslation,
      upperTranslation
    };

    const joint = this._world.createJoint(new planck.PrismaticJoint(
      jointDef,
      bodyA,
      bodyB,
      new planck.Vec2(anchorX, anchorY),
      new planck.Vec2(axisX, axisY)
    ));

    this._joints.push({ joint, type: 'prismatic', label, bodyA, bodyB });
    joint.setUserData({ label, type: 'prismatic' });
    return joint;
  }

  createDistanceJoint(config) {
    const {
      bodyA, bodyB,
      anchorAX, anchorAY,
      anchorBX, anchorBY,
      length,
      stiffness = 0, // 0 means rigid in some engines, but in planck frequencyHz is used
      damping = 0,
      label = 'distance'
    } = config;

    const anchorA = new planck.Vec2(anchorAX, anchorAY);
    const anchorB = new planck.Vec2(anchorBX, anchorBY);
    
    // In Planck.js, DistanceJoint uses frequencyHz and dampingRatio for stiffness/damping
    const jointDef = {
      length: length !== undefined ? length : planck.Vec2.distance(anchorA, anchorB),
      frequencyHz: stiffness,
      dampingRatio: damping
    };

    const joint = this._world.createJoint(new planck.DistanceJoint(
      jointDef,
      bodyA,
      bodyB,
      anchorA,
      anchorB
    ));

    this._joints.push({ joint, type: 'distance', label, bodyA, bodyB });
    joint.setUserData({ label, type: 'distance' });
    return joint;
  }

  createWeldJoint(config) {
    const { bodyA, bodyB, anchorX, anchorY, label = 'weld' } = config;
    const anchor = new planck.Vec2(anchorX, anchorY);
    
    const joint = this._world.createJoint(new planck.WeldJoint(
      {},
      bodyA,
      bodyB,
      anchor
    ));

    this._joints.push({ joint, type: 'weld', label, bodyA, bodyB });
    joint.setUserData({ label, type: 'weld' });
    return joint;
  }

  step() {
    const dt = (1 / 60) * this._speedMultiplier;
    this._world.step(dt, 8, 3);
    this._time += dt;
    this._stepCount++;

    this._stepCallbacks.forEach(cb => cb(this));
  }

  play() { this._playing = true; }
  pause() { this._playing = false; }
  
  reset() {
    // Destroy all joints and bodies
    for (let j = this._world.getJointList(); j; j = j.getNext()) {
      this._world.destroyJoint(j);
    }
    for (let b = this._world.getBodyList(); b; b = b.getNext()) {
      this._world.destroyBody(b);
    }
    
    this._bodies = [];
    this._joints = [];
    this._time = 0;
    this._stepCount = 0;
  }

  setSpeed(multiplier) {
    this._speedMultiplier = Math.max(0.1, Math.min(2.0, multiplier));
  }

  isPlaying() { return this._playing; }

  getBodies() {
    return this._bodies.map(b => this.getBodyData(b.body));
  }

  getJoints() {
    return this._joints.map(j => this.getJointData(j.joint));
  }

  getBodyData(body) {
    const bodyData = body.getUserData() || {};
    const position = body.getPosition();
    const velocity = body.getLinearVelocity();
    return {
      body,
      label: bodyData.label,
      userData: bodyData.userData,
      position: { x: position.x, y: position.y },
      angle: body.getAngle(),
      velocity: { x: velocity.x, y: velocity.y },
      angularVelocity: body.getAngularVelocity()
    };
  }

  getJointData(joint) {
    const jointData = joint.getUserData() || {};
    const dt = 1 / 60;
    const force = joint.getReactionForce(dt);
    
    return {
      joint,
      type: jointData.type,
      label: jointData.label,
      reactionForce: force ? force.length() : 0,
      reactionTorque: joint.getReactionTorque ? joint.getReactionTorque(dt) : 0,
      anchorA: joint.getAnchorA ? joint.getAnchorA() : { x: 0, y: 0 },
      anchorB: joint.getAnchorB ? joint.getAnchorB() : { x: 0, y: 0 }
    };
  }

  applyAerodynamicForce(body, config) {
    const { liftCoeff, dragCoeff, area, airDensity, windSpeed = {x: 0, y: 0} } = config;
    
    const velocity = body.getLinearVelocity();
    // Relative velocity to air
    const relV = new planck.Vec2(velocity.x - windSpeed.x, velocity.y - windSpeed.y);
    const speed = relV.length();
    
    if (speed < 0.001) return;
    
    const vDir = new planck.Vec2(relV.x / speed, relV.y / speed);
    
    // Drag acts opposite to velocity
    const dragMagnitude = 0.5 * airDensity * speed * speed * dragCoeff * area;
    const dragForce = new planck.Vec2(-vDir.x * dragMagnitude, -vDir.y * dragMagnitude);
    
    // Lift acts perpendicular to velocity (assuming standard wing profile, pointing up/down based on angle of attack)
    // For simplicity, we'll apply lift perpendicular to velocity in world space
    const liftMagnitude = 0.5 * airDensity * speed * speed * liftCoeff * area;
    const liftForce = new planck.Vec2(-vDir.y * liftMagnitude, vDir.x * liftMagnitude);
    
    body.applyForceToCenter(dragForce, true);
    body.applyForceToCenter(liftForce, true);
  }

  onCollision(callback) {
    this._collisionCallbacks.push(callback);
  }

  onStep(callback) {
    this._stepCallbacks.push(callback);
  }
}
