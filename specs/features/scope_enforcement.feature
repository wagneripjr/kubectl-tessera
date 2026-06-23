@e2e @cluster @cli
Feature: Scope enforcement
  A minted credential grants exactly the requested scope and nothing more.
  This is the core promise of tessera: the token is always narrow.

  # Acceptance criterion #1 — read-only scope
  @FR-001 @FR-004 @NFR-006
  Scenario Outline: a minted credential enforces the requested verb scope
    Given an operator requests "<verbs>" on "<resource>" in the session namespace
    When the operator mints the session
    Then the minted credential <outcome> to "<probe>" "<resource>"

    Examples:
      | verbs          | resource | probe  | outcome        |
      | get,list,watch | pods     | get    | is allowed     |
      | get,list,watch | pods     | delete | is not allowed |

  # Acceptance criterion #7 — cluster scope
  @FR-001 @cluster-scope
  Scenario: a cluster-scoped read session can read but not delete the resource
    Given an operator requests "get,list" on the cluster-scoped resource "nodes"
    When the operator mints the session
    Then the minted credential is allowed to "get" "nodes"
    And the minted credential is not allowed to "delete" "nodes"

  # Acceptance criterion #8 — resourceName narrowing
  @FR-002 @resource-name
  Scenario: a name-narrowed session acts only on the named object
    Given an operator requests "get" on "pods" named "foo" in the session namespace
    When the operator mints the session
    Then the minted credential is allowed to "get" the "pods" named "foo"
    And the minted credential is not allowed to "get" the "pods" named "bar"

  # FR-019 — explicit all-resources wildcard: one session, every resource type, narrow verbs
  @FR-019
  Scenario: an all-resources read session reaches every resource type but cannot mutate
    Given an operator requests "get,list,watch" on "*" in the session namespace
    When the operator mints the session
    Then the minted credential is allowed to "get" "pods"
    And the minted credential is allowed to "get" "configmaps"
    And the minted credential is allowed to "get" "services"
    And the minted credential is not allowed to "delete" "pods"

  # FR-019 — the wildcard never widens the boundary: a non-admin is still refused
  @FR-019
  Scenario: a non-admin operator cannot mint an all-resources session
    Given a limited operator who may only read "pods"
    When the limited operator requests "get" on "*" in the session namespace
    Then the mint is refused
    And no managed objects are created for that attempt
