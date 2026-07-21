import { ref } from 'vue'

// Cross-route hand-off for the secret player tour. The picker lives at /browse,
// while the actual tour starts only after the chosen /anime/:id view mounts.
// Module state is intentionally session-only: reload cancels the hidden tour.
export const playerGuideRequested = ref(false)
