@e2e @cluster @cli @cleanup
Feature: Session lifecycle and cleanup
  Every session is reclaimable through three independent layers: the token's own
  lifetime, the interactive exit trap, and the garbage-collection sweep. No failure
  mode leaves objects orphaned indefinitely.

  # Acceptance criterion #3 — TTL expiry
  @FR-006 @slow
  Scenario: a session credential stops working after its lifetime elapses
    Given an operator mints a read-only session with a very short lifetime
    Then the minted credential works immediately
    When the session lifetime elapses
    Then the minted credential no longer works

  # Acceptance criterion #4 — --exec cleanup
  @FR-009
  Scenario: leaving an interactive session removes its objects
    Given an operator mints an interactive read-only session
    When the operator leaves the interactive session
    Then the session's managed objects are gone
    And the session kubeconfig file is removed

  # Acceptance criterion #5 — crash recovery
  @FR-011
  Scenario: an abruptly terminated session is reclaimed by the sweep
    Given an operator mints an interactive read-only session
    When the session process is terminated abruptly
    And the session lifetime elapses
    And the garbage-collection sweep runs
    Then the session's managed objects are gone

  # Acceptance criterion #9 — rollback
  @FR-005
  Scenario: a failure partway through creation leaves no objects behind
    Given object creation will fail partway through
    When the operator mints a session
    Then no managed objects remain for that session

  # Acceptance criterion #10 — gc selectivity
  @FR-011 @NFR-005
  Scenario: the sweep removes only expired managed sessions
    Given an expired managed session exists
    And an unexpired managed session exists
    And an unmanaged role binding exists
    When the garbage-collection sweep runs
    Then the expired managed session is removed
    And the unexpired managed session remains
    And the unmanaged role binding remains
