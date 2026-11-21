# Set80 desktop fixture successor

The preserved desktop-normal-attempt1 failed two live test fixtures before their intended custody assertions. No product or independent reference is changed.

- `TestRecoverableRemoteProtectedOriginalLeaseAndMarkers` copied the frozen credentials whose public expiry is 1794790800000 (2026-11-16 01:00 UTC). The November 21 execution correctly failed the live owner expiry check. The lifecycle test now selects a one-hour future expiry; the separate pure reference test continues to validate the unchanged canonical credential bytes.
- `TestRecoverableRemoteCredentialChangeAfterClientResponseRetainsIntent` selected an HTTP test server with an empty CA. The remote configuration correctly requires HTTPS and explicit CA trust. The test now uses the actual httptest TLS server and its public certificate PEM. The original client-provenance, post-response file replacement and durable null-intent assertions remain unchanged.

Only these two external test copies changed. The current checkout preimages and original failure log are bound below. Root must verify preimages before installation and run the original focused normal/race/profile checks. No Go compile/test, server or runtime was executed in this source stage.
