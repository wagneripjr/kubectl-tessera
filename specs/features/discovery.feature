@e2e @manual @webhook
Feature: Scope discovery
  A dry run previews what the operator could scope to. Discovery is advisory only —
  when the cluster's authorizer cannot enumerate permissions, the preview says so
  rather than implying it is complete.

  # Acceptance criterion #11 — SSRR Incomplete surfaced
  # Runs only against a cluster with a non-enumerable (webhook) authorizer; excluded
  # from standard CI (see ADR-011). The notice-rendering path is also covered by a unit test.
  @FR-013
  Scenario: discovery warns when the authorizer cannot enumerate permissions
    Given a cluster whose authorizer cannot enumerate permissions
    When the operator previews the scope with a dry run
    Then a "discovery may be incomplete" warning is shown
    And the preview does not claim to be exhaustive
