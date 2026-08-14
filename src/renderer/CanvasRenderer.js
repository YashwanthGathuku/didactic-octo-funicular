import * as planck from 'planck-js';

export class CanvasRenderer {
  constructor(canvas, physicsWorld, failureAnalyzer) {
    this.canvas = canvas;
    this.ctx = canvas.getContext('2d');
    // We assume physicsWorld exposes the underlying planck.World as .world
    this.world = physicsWorld.world;
    this.physicsWorld = physicsWorld;
    this.failureAnalyzer = failureAnalyzer;

    this.PIXELS_PER_METER = 50;
    this.camera = { x: 0, y: 0, zoom: 1 };

    this.overlays = {
      forces: false,
      stress: false,
      velocity: false,
      trajectory: false,
      fbd: false
    };

    this.selectedBody = null;
    this.highlightedBody = null;
    this.highlightColor = '#ffeb3b';
    this.projectileHistory = new Map();

    this.resize();
  }

  resize() {
    // Make canvas fill its container
    this.canvas.width = this.canvas.parentElement.clientWidth;
    this.canvas.height = this.canvas.parentElement.clientHeight;
  }

  setCamera(x, y, zoom) {
    this.camera.x = x;
    this.camera.y = y;
    this.camera.zoom = zoom;
  }

  screenToWorld(screenX, screenY) {
    const hw = this.canvas.width / 2;
    const hh = this.canvas.height / 2;
    // Undo translation and scale
    // Canvas: x = hw + (worldX - camX) * scale
    //         y = hh - (worldY - camY) * scale
    const worldX = (screenX - hw) / (this.PIXELS_PER_METER * this.camera.zoom) + this.camera.x;
    const worldY = this.camera.y - (screenY - hh) / (this.PIXELS_PER_METER * this.camera.zoom);
    return { x: worldX, y: worldY };
  }

  worldToScreen(worldX, worldY) {
    const hw = this.canvas.width / 2;
    const hh = this.canvas.height / 2;
    const screenX = hw + (worldX - this.camera.x) * this.PIXELS_PER_METER * this.camera.zoom;
    const screenY = hh - (worldY - this.camera.y) * this.PIXELS_PER_METER * this.camera.zoom;
    return { x: screenX, y: screenY };
  }

  toggleOverlay(type) {
    if (this.overlays.hasOwnProperty(type)) {
      this.overlays[type] = !this.overlays[type];
    }
  }

  isOverlayActive(type) {
    return this.overlays[type] === true;
  }

  setOverlays(overlayMap) {
    this.overlays = { ...this.overlays, ...overlayMap };
  }

  getBodyAtPoint(screenX, screenY) {
    const worldPos = this.screenToWorld(screenX, screenY);
    const p = planck.Vec2(worldPos.x, worldPos.y);
    
    let foundBody = null;
    this.world.queryAABB(planck.AABB(p, p), (fixture) => {
      if (fixture.testPoint(p)) {
        foundBody = fixture.getBody();
        return false; // Stop at first found
      }
      return true;
    });
    return foundBody;
  }

  highlightBody(body, color) {
    this.highlightedBody = body;
    this.highlightColor = color;
  }

  clearHighlights() {
    this.highlightedBody = null;
  }

  render() {
    this.ctx.clearRect(0, 0, this.canvas.width, this.canvas.height);
    this._drawBackground();

    this.ctx.save();
    
    // Apply camera transform
    const hw = this.canvas.width / 2;
    const hh = this.canvas.height / 2;
    this.ctx.translate(hw, hh);
    this.ctx.scale(this.camera.zoom, this.camera.zoom);
    // Y is up in physics, down in canvas
    this.ctx.translate(-this.camera.x * this.PIXELS_PER_METER, this.camera.y * this.PIXELS_PER_METER);

    if (this.overlays.trajectory) {
      this._updateAndDrawTrajectories();
    }

    // Draw joints
    for (let joint = this.world.getJointList(); joint; joint = joint.getNext()) {
      this._drawJoint(joint);
    }

    // Draw bodies
    for (let body = this.world.getBodyList(); body; body = body.getNext()) {
      this._drawBody(body);
    }

    // Overlays
    if (this.overlays.forces) {
      this._drawForces();
    }
    if (this.overlays.velocity) {
      this._drawVelocities();
    }
    if (this.overlays.fbd && this.selectedBody) {
      this._drawFBD(this.selectedBody);
    }

    this.ctx.restore();
  }

  _drawBackground() {
    this.ctx.fillStyle = '#f4e4c1';
    this.ctx.fillRect(0, 0, this.canvas.width, this.canvas.height);

    // Grid lines
    this.ctx.strokeStyle = 'rgba(92, 64, 51, 0.1)';
    this.ctx.lineWidth = 1;
    const gridSize = 50 * this.camera.zoom;
    
    // Calculate offsets based on camera
    const offsetX = (this.canvas.width / 2 - this.camera.x * this.PIXELS_PER_METER * this.camera.zoom) % gridSize;
    const offsetY = (this.canvas.height / 2 + this.camera.y * this.PIXELS_PER_METER * this.camera.zoom) % gridSize;

    this.ctx.beginPath();
    for (let x = offsetX; x < this.canvas.width; x += gridSize) {
      this.ctx.moveTo(x, 0);
      this.ctx.lineTo(x, this.canvas.height);
    }
    for (let y = offsetY; y < this.canvas.height; y += gridSize) {
      this.ctx.moveTo(0, y);
      this.ctx.lineTo(this.canvas.width, y);
    }
    this.ctx.stroke();
  }

  _drawBody(body) {
    const pos = body.getPosition();
    const angle = body.getAngle();
    const userData = body.getUserData() || {};

    this.ctx.save();
    this.ctx.translate(pos.x * this.PIXELS_PER_METER, -pos.y * this.PIXELS_PER_METER);
    this.ctx.rotate(-angle);

    // FBD dimming
    if (this.overlays.fbd && this.selectedBody && this.selectedBody !== body) {
      this.ctx.globalAlpha = 0.2;
    }

    for (let fixture = body.getFixtureList(); fixture; fixture = fixture.getNext()) {
      const shape = fixture.getShape();
      const type = shape.getType();

      this.ctx.beginPath();
      if (type === 'polygon') {
        const vertices = shape.m_vertices;
        this.ctx.moveTo(vertices[0].x * this.PIXELS_PER_METER, -vertices[0].y * this.PIXELS_PER_METER);
        for (let i = 1; i < vertices.length; i++) {
          this.ctx.lineTo(vertices[i].x * this.PIXELS_PER_METER, -vertices[i].y * this.PIXELS_PER_METER);
        }
        this.ctx.closePath();
      } else if (type === 'circle') {
        const radius = shape.m_radius * this.PIXELS_PER_METER;
        this.ctx.arc(0, 0, radius, 0, Math.PI * 2);
      }

      // Base fill
      this.ctx.fillStyle = userData.color || '#d4c4a1';
      
      // Stress overlay color modification
      if (this.overlays.stress && this.failureAnalyzer) {
        const stressData = this.failureAnalyzer.getStressData();
        // Fallback or basic stress coloring logic
        // This assumes we track stress per body, or via joint forces
        this.ctx.fillStyle = this._getStressColor(body);
      }

      this.ctx.fill();

      // Cross-hatch ink effect
      this._drawCrossHatch();

      // Outline
      this.ctx.lineWidth = 2;
      this.ctx.strokeStyle = '#5c4033'; // sepia
      this.ctx.stroke();
      
      // Highlight
      if (this.highlightedBody === body || (this.overlays.fbd && this.selectedBody === body)) {
        this.ctx.lineWidth = 4;
        this.ctx.strokeStyle = this.highlightColor;
        this.ctx.stroke();
      }
      
      // Gear teeth
      if (type === 'circle' && (userData.type === 'gear' || userData.type === 'wheel')) {
         this._drawGearTeeth(shape.m_radius * this.PIXELS_PER_METER);
      }
    }
    this.ctx.restore();
  }

  _drawCrossHatch() {
    this.ctx.save();
    this.ctx.clip();
    this.ctx.strokeStyle = 'rgba(92, 64, 51, 0.15)';
    this.ctx.lineWidth = 1;
    this.ctx.beginPath();
    // Approximate bounds for simple crosshatch
    for (let i = -200; i < 200; i += 10) {
      this.ctx.moveTo(i, -200);
      this.ctx.lineTo(i + 400, 200);
    }
    this.ctx.stroke();
    this.ctx.restore();
  }

  _drawGearTeeth(radius) {
    this.ctx.save();
    const numTeeth = Math.floor(radius / 5);
    const toothHeight = 6;
    this.ctx.fillStyle = '#5c4033';
    for (let i = 0; i < numTeeth; i++) {
      const angle = (i / numTeeth) * Math.PI * 2;
      this.ctx.rotate(angle);
      this.ctx.beginPath();
      this.ctx.moveTo(radius - 2, -3);
      this.ctx.lineTo(radius + toothHeight, 0);
      this.ctx.lineTo(radius - 2, 3);
      this.ctx.fill();
      this.ctx.rotate(-angle);
    }
    this.ctx.restore();
  }

  _drawJoint(joint) {
    const type = joint.getType();
    const anchorA = joint.getAnchorA();
    const anchorB = joint.getAnchorB();
    
    // Draw rope/spring for distance joints
    if (type === 'distance-joint') {
      this.ctx.beginPath();
      this.ctx.moveTo(anchorA.x * this.PIXELS_PER_METER, -anchorA.y * this.PIXELS_PER_METER);
      // simple line for now, could be zigzag
      this.ctx.lineTo(anchorB.x * this.PIXELS_PER_METER, -anchorB.y * this.PIXELS_PER_METER);
      this.ctx.strokeStyle = '#5c4033';
      this.ctx.lineWidth = 2;
      this.ctx.stroke();
    }

    // Draw anchors
    this.ctx.fillStyle = '#b5a642'; // brass
    this.ctx.strokeStyle = '#5c4033';
    this.ctx.lineWidth = 1;
    
    const drawAnchor = (a) => {
      this.ctx.beginPath();
      this.ctx.arc(a.x * this.PIXELS_PER_METER, -a.y * this.PIXELS_PER_METER, 4, 0, Math.PI * 2);
      this.ctx.fill();
      this.ctx.stroke();
    };
    
    drawAnchor(anchorA);
    drawAnchor(anchorB);
  }

  _getStressColor(body) {
    // simplified stress color
    return '#d4c4a1';
  }

  _drawForces() {
    this.ctx.font = '14px Caveat, cursive';
    this.ctx.fillStyle = '#d4a843';
    this.ctx.strokeStyle = '#d4a843';

    for (let body = this.world.getBodyList(); body; body = body.getNext()) {
      if (body.getType() !== 'dynamic') continue;
      // Get force (Planck doesn't store net force directly, we approximate by mass * accel)
      const mass = body.getMass();
      const vel = body.getLinearVelocity();
      // simplified fake force vector for demo if real force isn't tracked
      const forceMag = vel.length() * mass;
      if (forceMag > 0.1) {
        const pos = body.getPosition();
        this._drawArrow(
          pos.x * this.PIXELS_PER_METER, -pos.y * this.PIXELS_PER_METER,
          (pos.x + vel.x) * this.PIXELS_PER_METER, -(pos.y + vel.y) * this.PIXELS_PER_METER,
          '#d4a843'
        );
      }
    }
  }

  _drawVelocities() {
    for (let body = this.world.getBodyList(); body; body = body.getNext()) {
      if (body.getType() !== 'dynamic') continue;
      const vel = body.getLinearVelocity();
      if (vel.length() > 0.1) {
        const pos = body.getPosition();
        this._drawArrow(
          pos.x * this.PIXELS_PER_METER, -pos.y * this.PIXELS_PER_METER,
          (pos.x + vel.x * 0.5) * this.PIXELS_PER_METER, -(pos.y + vel.y * 0.5) * this.PIXELS_PER_METER,
          '#00bcd4' // cyan
        );
      }
    }
  }

  _drawFBD(body) {
    // FBD logic
    const pos = body.getPosition();
    const mass = body.getMass();
    // Gravity
    this._drawArrow(
      pos.x * this.PIXELS_PER_METER, -pos.y * this.PIXELS_PER_METER,
      pos.x * this.PIXELS_PER_METER, -(pos.y - mass * 10 * 0.01) * this.PIXELS_PER_METER,
      '#6b1c23' // oxblood red
    );
  }

  _updateAndDrawTrajectories() {
    // Basic trajectory drawing
    this.ctx.strokeStyle = 'rgba(92, 64, 51, 0.5)';
    this.ctx.lineWidth = 1;
    this.ctx.setLineDash([5, 5]);

    for (let body = this.world.getBodyList(); body; body = body.getNext()) {
      const userData = body.getUserData() || {};
      if (userData.isProjectile) {
        const pos = body.getPosition();
        let history = this.projectileHistory.get(body) || [];
        history.push({ x: pos.x, y: pos.y });
        if (history.length > 50) history.shift();
        this.projectileHistory.set(body, history);

        this.ctx.beginPath();
        for (let i = 0; i < history.length; i++) {
          const pt = history[i];
          if (i === 0) this.ctx.moveTo(pt.x * this.PIXELS_PER_METER, -pt.y * this.PIXELS_PER_METER);
          else this.ctx.lineTo(pt.x * this.PIXELS_PER_METER, -pt.y * this.PIXELS_PER_METER);
        }
        this.ctx.stroke();
      }
    }
    this.ctx.setLineDash([]);
  }

  _drawArrow(fromx, fromy, tox, toy, color) {
    const headlen = 10; // length of head in pixels
    const dx = tox - fromx;
    const dy = toy - fromy;
    const angle = Math.atan2(dy, dx);
    this.ctx.strokeStyle = color;
    this.ctx.fillStyle = color;
    this.ctx.lineWidth = 2;
    this.ctx.beginPath();
    this.ctx.moveTo(fromx, fromy);
    this.ctx.lineTo(tox, toy);
    this.ctx.stroke();
    
    this.ctx.beginPath();
    this.ctx.moveTo(tox, toy);
    this.ctx.lineTo(tox - headlen * Math.cos(angle - Math.PI / 6), toy - headlen * Math.sin(angle - Math.PI / 6));
    this.ctx.lineTo(tox - headlen * Math.cos(angle + Math.PI / 6), toy - headlen * Math.sin(angle + Math.PI / 6));
    this.ctx.lineTo(tox, toy);
    this.ctx.fill();
  }
}
