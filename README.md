# National Single Window for Trade - Sri Lanka

<!-- markdownlint-disable-next-line MD033 -->
<p align="center"><img src="portals/apps/trader-app/public/branding/one-trade-logo.png" alt="OneTrade — Trade . Connect . Grow" width="360"></p>

[![Built on OpenNSW Core](https://img.shields.io/badge/built%20on-OpenNSW%2Fcore-2EA44F)](https://github.com/OpenNSW/core)
[![Artifacts from one-trade-artifacts](https://img.shields.io/badge/artifacts%20from-OpenNSW%2Fone--trade--artifacts-4169E1)](https://github.com/OpenNSW/one-trade-artifacts)
[![Last Commit](https://img.shields.io/github/last-commit/OpenNSW/nsw-srilanka?logo=github)](https://github.com/OpenNSW/nsw-srilanka/commits/main)
[![Open Issues](https://img.shields.io/github/issues/OpenNSW/nsw-srilanka?logo=github)](https://github.com/OpenNSW/nsw-srilanka/issues)

[![Temporal](https://img.shields.io/badge/Temporal-1.28-000000?logo=temporal&logoColor=white)](https://temporal.io)
[![ThunderID](https://img.shields.io/badge/ThunderID-1.1-FF7300)](https://thunderid.dev)
[![OpenTelemetry](https://img.shields.io/badge/OpenTelemetry-000000?logo=opentelemetry&logoColor=white)](https://opentelemetry.io)

An integrated platform to enhance trade competitiveness through digital transformation.

`nsw-srilanka` is the application repository for **Sri Lanka's National Single Window (NSW) for Trade**.

It is built on [OpenNSW/core](https://github.com/OpenNSW/core), with workflows and forms from [OpenNSW/one-trade-artifacts](https://github.com/OpenNSW/one-trade-artifacts).

## Why it matters for Sri Lanka

- **For traders: one portal for everything.** Submit applications to every agency, pay the fees, and track the status of each agency's tasks, all in one place.
- **For agencies: their own portal.** Each agency's officers review traders' applications in a portal with the agency's own branding.

Less paperwork, faster clearance, lower costs.

## Architecture

![Architecture of the National Single Window: traders and CHAs use TNSW and agency officers use the CDA, FCAU, NPQS, SLTB and EFC agencies on the TNSW platform; they rely on GovTech-run infrastructure (ThunderID, Temporal, email, audit, PostgreSQL and S3 object storage) and connect to SLPA's CMS, GovPay, SLC Edge and ASYCUDA, an SMS provider, and the IPPC hub](docs/diagrams/architecture.svg)

---

## How to Run Locally

You need Go 1.27+, Docker with Compose v2, Node.js 24.15+, pnpm 12.16+ and [golangci-lint v2](https://golangci-lint.run).

1. **Clone the artifacts** next to this repo. The agencies load their workflows and forms from it:

   ```bash
   git clone https://github.com/OpenNSW/one-trade-artifacts.git ../one-trade-artifacts
   ```

2. **Set up** (installs tools and creates local config files from the examples):

   ```bash
   make setup
   ```

3. **Start** the database, identity provider, Temporal, the NSW API and Trader Portal, and the agencies:

   ```bash
   make dev
   ```

4. **Open** the Trader Portal at <http://localhost:5173>. The API runs at <http://localhost:8080> (`/health` returns `ok`).

Each agency runs its own API and officer portal next to NSW:

| Agency                                   | Portal                  | API                     |
| ---------------------------------------- | ----------------------- | ----------------------- |
| National Plant Quarantine Service (NPQS) | <http://localhost:5174> | <http://localhost:8081> |
| Food Control Administration Unit (FCAU)  | <http://localhost:5175> | <http://localhost:8082> |
| Coconut Development Authority (CDA)      | <http://localhost:5176> | <http://localhost:8083> |
| Export Facilitation Center (EFC)         | <http://localhost:5178> | <http://localhost:8085> |
| Sri Lanka Tea Board (SLTB)               | <http://localhost:5179> | <http://localhost:8086> |

Changes to the NSW API and Trader Portal reload automatically. The agencies don't; rebuild one after a change with `docker compose up -d --build <agency>-api <agency>-portal`. Each agency's setup is in `configs/agency/<agency>/README.md`, and how agency mode works is in [docs/agency.md](docs/agency.md).

Run `make help` to see all commands.

> [!NOTE]
> This setup is for local development only.

---

## Database migrations

Migrations are SQL files in `migrations/`, applied automatically to the NSW and agency databases when the stack starts.

| Task                    | Command                                                                                |
| ----------------------- | -------------------------------------------------------------------------------------- |
| Create a new migration  | `make migration name=add_example_table`                                                |
| Apply a new migration   | `docker compose up -d --build migrate cda-init efc-init npqs-init sltb-init fcau-init` |
| See applied and pending | `docker compose run --rm migrate migrate status`                                       |
| Roll back the latest    | `docker compose run --rm migrate migrate down`                                         |
| Reset the database      | `make clean`, then `make dev`                                                          |

Each file has an `@UP` and a reversible `@DOWN` block. Never edit a migration that has already been applied. See [CONTRIBUTING.md](CONTRIBUTING.md#migration-check-migrationsyml) for details.

---

## Configuration Reference

`make setup` creates these files from their `.example` versions. Edit them for your environment.

| File                                       | What it holds                               |
| ------------------------------------------ | ------------------------------------------- |
| `.env`                                     | Secrets and Docker port settings            |
| `idp/.env`                                 | Identity provider settings                  |
| `configs/companies.json`                   | Sample companies and traders                |
| `configs/config.yaml`                      | Server settings, for running without Docker |
| `configs/config.docker.yaml`               | Server settings, for running with Docker    |
| `portals/apps/trader-app/public/config.js` | Trader Portal settings                      |

To write or change workflows and forms, see [WORKFLOW_GUIDE.md](docs/WORKFLOW_GUIDE.md).
