@e2e @cluster @cli
Feature: Session inventory, previews and precondition errors
  An operator can see which sessions are currently active, preview what a mint would
  create before committing to it, and — when they lack the permissions tessera needs to
  create the objects — be told precisely what is missing instead of a raw API error.

  # FR-012 — list active sessions (empty case)
  @FR-012
  Scenario: listing sessions when none are active yields an empty inventory
    Given no tessera sessions are active
    When the operator lists active sessions in machine-readable form
    Then the inventory is empty

  # FR-012 — list active sessions (populated case)
  @FR-012 @FR-015
  Scenario: an active session appears in the inventory
    Given an operator requests a read-only "pods" session for non-interactive use
    And the operator mints the session in print-kubeconfig mode
    When the operator lists active sessions in machine-readable form
    Then the inventory includes the active session with its owner and expiry

  # FR-010 — dry-run previews the intended objects and creates nothing
  @FR-010
  Scenario: a dry run previews the intended objects without creating them
    Given an operator requests a read-only "pods" session for non-interactive use
    When the operator previews the session with a dry run
    Then the intended objects are described on the primary output
    And no managed objects are created for that attempt

  # FR-016 — clear error when the operator cannot create the RBAC objects
  @FR-016
  Scenario: minting is refused with a clear message when the operator cannot create the objects
    Given a limited operator who may only read "pods"
    When the limited operator mints exactly the read-only "pods" scope they are allowed
    Then the mint is refused
    And the operator is told which create permission is missing
    And no managed objects are created for that attempt
