@e2e @cluster @cli
Feature: Multi-namespace and all-namespaces sessions
  A session can span more than one namespace. An operator either names an explicit
  set of namespaces (FR-017) or asks for every namespace at once (FR-018). In both
  cases the operator receives a single credential whose reach is exactly what was
  asked for — never wider. The all-namespaces grant is the largest scope tessera can
  mint, so it is gated cluster-wide: an operator who cannot already read the resource
  in every namespace cannot mint it.

  # FR-017 — an explicit list of namespaces yields one credential that reaches each
  # of them and nothing more. Atomic cleanup across the set is namespace-agnostic and
  # is covered by lifecycle_cleanup.feature (gc sweeps all namespaces by label).
  @FR-017 @multi-namespace
  Scenario: a multi-namespace session reaches each requested namespace and no others
    Given an operator requests "get,list" on "pods" across two namespaces
    When the operator mints the session
    Then the minted credential is allowed to "get" "pods" in each requested namespace
    And the minted credential is not allowed to "get" "pods" in an unrequested namespace

  # FR-018 — the all-namespaces wildcard grants the resource cluster-wide, including
  # namespaces that do not exist yet at mint time. It is read-only here, so it must
  # not grant the unrequested verb anywhere.
  @FR-018 @all-namespaces
  Scenario: an all-namespaces session reaches every namespace including new ones
    Given an operator requests "get,list" on "pods" in all namespaces
    When the operator mints the session
    Then the minted credential is allowed to "get" "pods" in the session namespace
    And the minted credential is allowed to "get" "pods" in a namespace created afterwards
    And the minted credential is not allowed to "delete" "pods" in the session namespace

  # FR-018 — the wildcard is self-limiting via the cluster-wide SSAR gate: an operator
  # who may only read in one namespace cannot mint an all-namespaces session, and the
  # refusal creates nothing anywhere (no leaked ClusterRoleBinding).
  @FR-018 @all-namespaces
  Scenario: an operator limited to one namespace cannot mint an all-namespaces session
    Given a limited operator who may only read "pods"
    When the limited operator requests "get" on "pods" in all namespaces
    Then the mint is refused
    And no managed objects are created anywhere for that attempt
