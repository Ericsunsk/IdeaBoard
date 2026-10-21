package postgres

import (
	"context"
	"errors"
	"testing"

	"ideaboard/api/internal/db/dbutil"
	"ideaboard/api/internal/tenancy"
)

func TestBillingMemberMutationErrorsPreserveMembership(t *testing.T) {
	for _, mutation := range []string{"role", "detach"} {
		t.Run(mutation, func(t *testing.T) {
			db := newTenancyTestDB(t)
			ctx := context.Background()
			_, accounts, owner, admin, _, account := handoverFixture(t, db, ctx)
			outsider := createTenancyTestUser(t, db)
			if _, err := accounts.AddBillingAccountMember(ctx, account.InternalID, owner, admin, tenancy.BillingRoleAdmin); err != nil {
				t.Fatal(err)
			}
			for _, tc := range []struct {
				name          string
				actor, target int64
				newRole       string
				want          error
			}{
				{"missing actor", outsider, admin, tenancy.BillingRoleOwner, tenancy.ErrBillingForbidden},
				{"missing target", owner, outsider, tenancy.BillingRoleOwner, tenancy.ErrBillingNotFound},
				{"missing actor and target", outsider, outsider, tenancy.BillingRoleOwner, tenancy.ErrBillingNotFound},
				{"last owner self", owner, owner, tenancy.BillingRoleAdmin, tenancy.ErrBillingLastOwner},
				{"last owner by admin", admin, owner, tenancy.BillingRoleAdmin, tenancy.ErrBillingLastOwner},
			} {
				t.Run(tc.name, func(t *testing.T) {
					var err error
					if mutation == "detach" {
						err = accounts.RemoveBillingAccountMember(ctx, account.InternalID, tc.actor, tc.target)
					} else {
						_, err = accounts.UpdateBillingAccountMemberRole(ctx, account.InternalID, tc.actor, tc.target, tc.newRole)
					}
					if !errors.Is(err, tc.want) {
						t.Fatalf("mutation = %v, want %v", err, tc.want)
					}
					for userID, wantRole := range map[int64]string{owner: tenancy.BillingRoleOwner, admin: tenancy.BillingRoleAdmin} {
						role, exists, err := accounts.GetBillingAccountRole(ctx, account.InternalID, userID)
						if err != nil || !exists || role != wantRole {
							t.Fatalf("member %d = %q, %v, %v; want %q", userID, role, exists, err, wantRole)
						}
					}
				})
			}
		})
	}
}

func TestBillingMemberMutationsJoinAmbientTransaction(t *testing.T) {
	for _, mutation := range []string{"role", "detach"} {
		t.Run(mutation, func(t *testing.T) {
			db := newTenancyTestDB(t)
			ctx := context.Background()
			_, accounts, owner, admin, _, account := handoverFixture(t, db, ctx)
			if _, err := accounts.AddBillingAccountMember(ctx, account.InternalID, owner, admin, tenancy.BillingRoleAdmin); err != nil {
				t.Fatal(err)
			}
			tx, err := db.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			carrier := dbutil.ContextWithAmbientTx(ctx, tx)
			if mutation == "detach" {
				err = accounts.RemoveBillingAccountMember(carrier, account.InternalID, admin, admin)
			} else {
				_, err = accounts.UpdateBillingAccountMemberRole(carrier, account.InternalID, owner, admin, tenancy.BillingRoleOwner)
			}
			if err != nil {
				t.Fatal(err)
			}
			if mutation == "role" {
				var got string
				if err := tx.QueryRow(ctx, `SELECT role FROM billing_account_members WHERE billing_account_id = $1 AND user_id = $2`, account.InternalID, admin).Scan(&got); err != nil || got != tenancy.BillingRoleOwner {
					t.Fatalf("transaction role = %q, %v", got, err)
				}
			} else {
				var count int
				if err := tx.QueryRow(ctx, `SELECT count(*) FROM billing_account_members WHERE billing_account_id = $1 AND user_id = $2`, account.InternalID, admin).Scan(&count); err != nil || count != 0 {
					t.Fatalf("transaction membership count = %d, %v", count, err)
				}
			}
			if err := tx.Rollback(ctx); err != nil {
				t.Fatal(err)
			}
			role, exists, err := accounts.GetBillingAccountRole(ctx, account.InternalID, admin)
			if err != nil || !exists || role != tenancy.BillingRoleAdmin {
				t.Fatalf("rolled-back member = %q, %v, %v", role, exists, err)
			}
		})
	}
}
