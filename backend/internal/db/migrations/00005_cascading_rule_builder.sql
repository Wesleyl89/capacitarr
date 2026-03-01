-- Migration 00005: Refactor protection_rules for cascading rule builder
-- Adds integration_id FK and effect column, migrates existing type+intensity data

-- Add integration_id FK (nullable for backward compat with existing global rules)
ALTER TABLE protection_rules ADD COLUMN integration_id INTEGER REFERENCES integration_configs(id) ON DELETE CASCADE;

-- Add effect column (replaces type + intensity)
ALTER TABLE protection_rules ADD COLUMN effect TEXT NOT NULL DEFAULT '';

-- Migrate existing data: combine type + intensity into effect
UPDATE protection_rules SET effect = CASE
    WHEN type = 'protect' AND intensity = 'absolute' THEN 'always_keep'
    WHEN type = 'protect' AND intensity = 'strong'   THEN 'prefer_keep'
    WHEN type = 'protect' AND intensity = 'slight'   THEN 'lean_keep'
    WHEN type = 'target'  AND intensity = 'slight'   THEN 'lean_remove'
    WHEN type = 'target'  AND intensity = 'strong'   THEN 'prefer_remove'
    WHEN type = 'target'  AND intensity = 'absolute' THEN 'always_remove'
    ELSE 'lean_keep'
END;

-- Note: type and intensity columns are kept for backward compatibility during transition.
-- They can be dropped in a future migration once the new system is stable.
