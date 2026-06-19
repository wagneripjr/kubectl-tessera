@e2e @cluster @cli
Feature: Pre-flight authorization gate
  An over-ask is refused before any object is created. The gate is the operator's
  fast, clear feedback; for a non-admin operator the API server enforces the same
  boundary at binding creation.

  # Acceptance criterion #2 — pre-flight refuses over-ask (non-admin)
  @FR-003
  Scenario: requesting a verb the operator lacks is refused before creating anything
    Given a limited operator who may only read "pods"
    When the limited operator requests "delete" on "pods" in the session namespace
    Then the mint is refused
    And the allowed and denied parts of the requested scope are reported
    And no managed objects are created for that attempt
