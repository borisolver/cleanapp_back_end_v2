# Physical Location Contact Email Inference (Proposal)

This document outlines a free, robust approach for inferring contact emails for physical locations (e.g., campus buildings) using OpenStreetMap (OSM) and public data. It complements the existing digital-contact inference pipeline.

## Data sources (free)
- **OpenStreetMap (primary)**
  - Overpass API (read-only) to query objects around a coordinate or polygon.
  - OSM tags often include `contact:email`, `email`, `operator`, `owner`, `brand`, and website URLs.
  - OSM `addr:*` fields and `name`/`operator` labels help derive domains.
- **Institutional websites (fallback)**
  - Fetch public homepages returned by OSM (e.g., `website` tag) and parse mailto links or `Contact` pages with a small HTML scraper.
- **Public campus directories (optional)**
  - Many universities expose JSON/CSV directory endpoints or accessible HTML that can be scraped politely for role-based emails (e.g., `facilities@ucla.edu`).

## Inference pipeline
1. **Locate the feature**
   - Reverse-geocode report coordinates with OSM Nominatim to get campus/building names and OSM IDs.
   - Run an Overpass query for features within a small radius (e.g., 150–250m) ordered by distance that match expected facility types: `amenity=*`, `building=*`, `office=*`, `university`, `school`, `hospital`, `public_transport`, `shop`, `tourism`, etc.
   - Prefer features whose `name`/`operator`/`brand` strings overlap with the report text (e.g., "UCLA Law" matches `name~"UCLA"` and `name~"Law"`).

2. **Direct email extraction (OSM tags)**
   - If any candidate has `contact:email` or `email`, collect them immediately.
   - If missing, use `website`, `contact:website`, `operator:website`, or `brand:wikipedia` URLs to crawl for emails.

3. **Domain inference (for campuses/businesses)**
   - Derive a canonical domain from OSM tags:
     - `website` or `operator:website` (strip path and protocol).
     - If absent, construct from operator name via a ruleset (e.g., `University of California, Los Angeles` → `ucla.edu`; `City of Santa Monica` → `santamonica.gov`).
   - Generate role-based emails using campus-aware heuristics:
     - Default set: `info@<domain>`, `support@<domain>`, `contact@<domain>`, `help@<domain>`.
     - Facilities/custodial: `facilities@<domain>`, `maintenance@<domain>`, `custodian@<domain>`, `grounds@<domain>`.
     - Safety/security: `security@<domain>`, `police@<domain>`, `publicsafety@<domain>`.
     - Accessibility: `ada@<domain>` or `accessibility@<domain>`.
   - If the OSM feature name includes a subdivision (e.g., `School of Law`), prepend it when forming custodial contacts: `law@<domain>`, `custodian-law@<domain>`, `facilities-law@<domain>`.

4. **Hierarchy-aware expansion**
   - Walk up the place hierarchy from the OSM reverse-geocode result:
     - Specific feature (building) → campus (e.g., `UCLA`) → operator/owner (e.g., `University of California` or `City of Los Angeles`).
   - For each level, apply domain inference and generate role-based candidates. This yields multiple responsible parties (e.g., building facilities, campus facilities, city public works).

5. **Scoring and de-duplication**
   - Score candidates by evidence source:
     1. OSM-tagged emails (exact) — highest.
     2. Emails scraped from linked websites.
     3. Role-based heuristics on feature domain.
     4. Role-based heuristics on parent domain.
   - Remove duplicates, normalize casing, and keep the top 3–5 distinct addresses for notification.

6. **Safety and quality controls**
   - Validate email syntax (RFC 5322 regex) and filter obvious placeholders (e.g., `test@`, `example@`).
   - Enforce a allowlist for academic/government TLDs when inferring from place names (`.edu`, `.gov`, `.ca.gov`, `.org`).
   - Rate-limit Overpass/HTTP calls and cache results per `(lat, lon)` and feature name to respect service limits.

## Example (UCLA Law School report)
- Reverse-geocode → `UCLA School of Law` building, operator `University of California, Los Angeles`, website `https://law.ucla.edu`.
- OSM lacks direct email.
- Domain inference yields `law.ucla.edu` (building) and `ucla.edu` (campus/owner).
- Generated candidates:
  - `contact@law.ucla.edu`, `facilities@law.ucla.edu`, `custodian-law@ucla.edu`.
  - Campus-level: `facilities@ucla.edu`, `security@ucla.edu`, `accessibility@ucla.edu`.
- Score by specificity; keep top 4–5 unique addresses for the notification batch.

## Implementation notes
- Add an `osm-contact-inference` module that:
  - Accepts `(lat, lon, report_text)` and returns ranked emails with provenance.
  - Uses an Overpass client (e.g., `requests` + Overpass QL) with a small template query.
  - Integrates with existing analysis pipeline to populate `inferred_contact_emails` for physical reports, tagging source (`osm_tag`, `website_scrape`, `heuristic_feature`, `heuristic_parent`).
- Provide unit tests with canned Overpass responses to keep deterministic.
- Keep the system toggleable via config flag to guard against rate-limits.
