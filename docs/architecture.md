# Architecture

ARAM separates the product shell from both application-level and full-system
emulation.

```text
                       frontend
             menus / input / audio / tools
                           |
                     core.Machine
                      /          \
             WIPI application   firmware system
              ARM + HLE          ARM + devices
                 |                    |
         carrier/device profile database
```

## Package boundaries

- `cmd/aram`: process entry point and CLI.
- `internal/frontend`: desktop command model, menu shell, recent files, and
  presentation state.
- `internal/core`: machine lifecycle, framebuffer/audio/input contracts, and
  state boundaries.
- `internal/cpu`: replaceable ARM execution interface.
- `internal/loader`: format detection and safe container parsers.
- `internal/profile`: carrier, manufacturer, device, and title configuration.

The frontend must remain usable when no execution backend is compiled. This
allows inspection, configuration, and compatibility reporting to work
independently.

## Planned backends

### Application mode

1. Parse DAT/ABHS/EADS or another supported package.
2. Map the native image into a 32-bit guest address space.
3. Execute ARM/Thumb through a CPU backend.
4. Resolve WIPI standard and OEM services through profile-selected HLE.
5. Present framebuffer, audio, input, storage, and timer state through the
   shared machine contract.

### System mode

1. Load a user-supplied firmware/flash image or validated memory snapshot.
2. Instantiate the selected SoC/device profile.
3. Run progressively from AMSS snapshot boot toward QCSBL/OEMSBL cold boot.
4. Model only safe virtual telephony; never bridge an emulated baseband to a
   real mobile network by default.

## Determinism

Virtual time, seeded randomness, virtual storage, input queues, and network
responses are part of machine state. Save states must include backend context,
RAM, HLE objects, device state, timers, audio sequencing, filesystem metadata,
and profile identity.
