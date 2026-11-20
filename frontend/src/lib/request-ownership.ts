// Async callbacks must not act on a response from a request the user has
// already superseded (#346, #349, #351). One owner per interaction sequence:
// every new selection/request takes ownership, and older completions are
// ignored for results, errors and busy state.
export function requestOwner() {
  let generation = 0;
  return {
    next(): number {
      return ++generation;
    },
    isCurrent(id: number): boolean {
      return generation === id;
    },
  };
}
