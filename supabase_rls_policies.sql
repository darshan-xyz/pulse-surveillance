-- Enable Row Level Security on cameras table
alter table cameras enable row level security;

-- Drop existing policies if they exist (to avoid conflicts)
drop policy if exists "Users can insert their own cameras" on cameras;
drop policy if exists "Users can view their own cameras" on cameras;
drop policy if exists "Users can update their own cameras" on cameras;
drop policy if exists "Users can delete their own cameras" on cameras;

-- Create RLS policies for cameras table
create policy "Users can insert their own cameras" on cameras
  for insert with check (auth.uid() = user_id);

create policy "Users can view their own cameras" on cameras
  for select using (auth.uid() = user_id);

create policy "Users can update their own cameras" on cameras
  for update using (auth.uid() = user_id);

create policy "Users can delete their own cameras" on cameras
  for delete using (auth.uid() = user_id);




