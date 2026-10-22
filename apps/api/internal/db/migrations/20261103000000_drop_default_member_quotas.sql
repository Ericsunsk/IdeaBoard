-- Drop the account default new-member quota (#953 retires #947).
--
-- New Organization Members default to unlimited and invites never gate on a
-- default; the per-member (Billing Account, User) quota rows and immutable
-- settlement history are preserved (only the default policy table leaves).
-- destructive: approved
DROP TABLE billing_account_default_member_quotas;
