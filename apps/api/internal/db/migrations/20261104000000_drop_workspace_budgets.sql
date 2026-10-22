-- Drop the Workspace shared consumption budget (#954 retires #945).
--
-- Budgets were a per-Workspace cumulative cap, never a wallet or balance
-- reservation; settlement no longer checks them and the API/UI no longer
-- exposes them. The per-member (Billing Account, User) quota rows and
-- immutable settlement history are preserved (only the budget table leaves).
-- destructive: approved
DROP TABLE workspace_budgets;
