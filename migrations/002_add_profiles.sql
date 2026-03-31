-- Add user profiles functionality
-- Migration 002: Add profiles table and update sessions table

-- Create profiles table
CREATE TABLE IF NOT EXISTS profiles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    style TEXT NOT NULL DEFAULT '',
    constraints TEXT NOT NULL DEFAULT '',
    context TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    is_default BOOLEAN DEFAULT FALSE
);

-- Create indexes for faster lookups
CREATE INDEX IF NOT EXISTS idx_profiles_name ON profiles(name);
CREATE INDEX IF NOT EXISTS idx_profiles_created_at ON profiles(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_profiles_is_default ON profiles(is_default) WHERE is_default = TRUE;

-- Add profile_id column to sessions table
ALTER TABLE sessions 
ADD COLUMN IF NOT EXISTS profile_id UUID REFERENCES profiles(id) ON DELETE SET NULL;

-- Create index for session profile lookups
CREATE INDEX IF NOT EXISTS idx_sessions_profile_id ON sessions(profile_id);

-- Insert a default profile for backward compatibility
INSERT INTO profiles (id, name, style, constraints, context, is_default, created_at, updated_at)
VALUES (
    '00000000-0000-0000-0000-000000000000',
    'Default',
    'Respond in a helpful, friendly, and professional manner.',
    'Be accurate, concise, and avoid harmful content.',
    'You are an AI assistant helping users with their questions.',
    TRUE,
    NOW(),
    NOW()
) ON CONFLICT (id) DO NOTHING;