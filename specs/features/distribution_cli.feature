@e2e @cluster @cli
Feature: Non-interactive kubeconfig output
  For automated callers (such as an AI agent), tessera prints a path to a ready-to-use
  scoped kubeconfig and nothing else on its primary output, so it composes cleanly into
  a shell pipeline.

  # Acceptance criterion #6 — --print-kubeconfig hygiene
  @FR-008 @FR-014 @NFR-008
  Scenario: printing a kubeconfig emits only the path and yields a usable credential
    Given an operator requests a read-only "pods" session for non-interactive use
    When the operator mints the session in print-kubeconfig mode
    Then only the kubeconfig path is written to the primary output
    And the produced kubeconfig grants the requested read access
    And the session audit details are written only to the diagnostic output
