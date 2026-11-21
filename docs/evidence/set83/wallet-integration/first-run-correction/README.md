# First integration-run correction

The original stage59e5878c, corpusb62572b6 and failed root attempt1 stay unchanged.
This external successor uses exact installed/root-formatted preimages.

- Product frontend defect: the selected fixture root public key is a 112-byte
  Base58 extended public key and the gym key is a compressed 33-byte key (66
  lowercase hex characters). The page wrongly used its 64-hex identifier test
  for both. Correct only those shapes; the native bridge still validates the
  exact independently selected public keys. DOM controls add malformed root
  and compressed-key cases and improve the failed-login diagnostic.
- Corpus consumer layering: `request-wrong-view` includes the producer's
  original live View; the stateless parser accepts shape/config and cannot know
  that current View. After parsing, this contextual case now invokes the actual
  production dailyWalletStep with that original View and asserts exact
  stale_view plus zero control/read effects. No parser, service, expected case
  or independent byte is changed; no case is skipped.
- Compilation: remove one unused path/filepath import in the selected native
  owner. No lifecycle or other product behavior changes.

No Go, JavaScript or helper execution was performed by this lane. Root owns
installation/formatting and the next normal/DOM qualification. The separate
Wails workspace setup failure is not addressed by this source correction.
