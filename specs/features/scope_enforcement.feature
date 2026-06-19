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
