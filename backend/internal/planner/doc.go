// Package planner searches for production plans that would make CBP likely
// assign a target origin. A plan is a single or double move of operation
// groups to plants able to perform them. For each move the planner renders the
// variant's facts deterministically from the structured product, asks the
// engine for its origin, keeps moves predicted to reach the target, has the
// verifier check them against earlier rulings, and ranks them by duty effect
// minus added cost (user inputs, illustrative). Plans are
// "precedent-supported", never approved or guaranteed.
package planner
