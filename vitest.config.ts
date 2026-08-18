import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    // The API client is environment-free by construction: it takes fetch from
    // the global and reads one cookie. Running it under node rather than a
    // simulated DOM keeps the tests about the client's behaviour instead of
    // about the fidelity of the simulation.
    environment: 'node',
    include: ['src/**/*.test.ts', 'src/**/*.test.tsx'],
  },
});
