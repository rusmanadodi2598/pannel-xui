// Package usersvc test covers identity and balance orchestration (AGENTS.md §2.1).
//
// @file      internal/service/user/user_test.go
// @for       Unit tests: EnsureUser, Get, Balance, Debit/Credit, FR-11 admin delegation.
// @uses      testing, context, errors, github.com/kentangtech/bot-order/internal/domain,
// internal/repository/postgres
// @reason    User/balance is the trust boundary of every order flow — every
// method must delegate with correct wrapping (M7 hardening, coverage gap 0%).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package usersvc

import (
	"context"
	"errors"
	"testing"

	"github.com/kentangtech/bot-order/internal/domain"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

var errStore = errors.New("store boom")

type fakeStore struct {
	user        *postgres.User
	err         error
	found       *postgres.User
	balance     domain.Money
	debited     []string // order ids
	credited    []string
	banned      []int64
	unbanned    []int64
	list        []int64
	count       int64
	debitResult domain.Money
}

func (f *fakeStore) FindOrCreate(_ context.Context, _ int64, _, _ string) (*postgres.User, error) {
	return f.found, f.err
}
func (f *fakeStore) GetByTelegramID(context.Context, int64) (*postgres.User, error) {
	return f.user, f.err
}
func (f *fakeStore) Debit(_ context.Context, _ int64, _ domain.Money, orderID string) (domain.Money, error) {
	f.debited = append(f.debited, orderID)
	return f.debitResult, f.err
}
func (f *fakeStore) Credit(_ context.Context, _ int64, _ domain.Money, orderID string) (domain.Money, error) {
	f.credited = append(f.credited, orderID)
	return f.balance, f.err
}
func (f *fakeStore) SetBanned(_ context.Context, tgID int64, banned bool) error {
	if banned {
		f.banned = append(f.banned, tgID)
	} else {
		f.unbanned = append(f.unbanned, tgID)
	}
	return f.err
}
func (f *fakeStore) ListTelegramIDs(context.Context, int, int) ([]int64, error) {
	return f.list, f.err
}
func (f *fakeStore) CountUsers(context.Context) (int64, error) {
	return f.count, f.err
}

func TestEnsureUser_GivenFirstContact_ThenDelegates(t *testing.T) {
	u := &postgres.User{ID: 5, TelegramID: 42}
	svc := New(&fakeStore{found: u})

	got, err := svc.EnsureUser(context.Background(), 42, "ada", "Ada")
	if err != nil {
		t.Fatalf("EnsureUser: %v", err)
	}
	if got != u {
		t.Errorf("EnsureUser = %v, want the stored row", got)
	}
}

func TestEnsureUser_GivenStoreError_ThenPropagates(t *testing.T) {
	svc := New(&fakeStore{err: errStore})
	if _, err := svc.EnsureUser(context.Background(), 1, "x", "X"); !errors.Is(err, errStore) {
		t.Fatalf("EnsureUser err = %v, want errStore", err)
	}
}

func TestGet_GivenUser_ThenReturns(t *testing.T) {
	u := &postgres.User{ID: 3, TelegramID: 77}
	svc := New(&fakeStore{user: u})

	got, err := svc.Get(context.Background(), 77)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != 3 {
		t.Errorf("Get = %+v, want user 3", got)
	}
}

func TestBalance_GivenUser_ThenReturnsBalance(t *testing.T) {
	svc := New(&fakeStore{user: &postgres.User{Balance: domain.Money(15000)}})

	bal, err := svc.Balance(context.Background(), 9)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if bal != 15000 {
		t.Errorf("Balance = %d, want 15000", bal)
	}
}

func TestBalance_GivenStoreError_ThenWrapped(t *testing.T) {
	svc := New(&fakeStore{err: errStore})
	if _, err := svc.Balance(context.Background(), 9); !errors.Is(err, errStore) {
		t.Fatalf("Balance err = %v, want errStore", err)
	}
}

func TestDebit_GivenSufficient_ThenNewBalanceAndOrderID(t *testing.T) {
	store := &fakeStore{debitResult: domain.Money(5000)}
	svc := New(store)

	bal, err := svc.Debit(context.Background(), 1, domain.Money(10000), "KTS-1-VPN")
	if err != nil {
		t.Fatalf("Debit: %v", err)
	}
	if bal != 5000 {
		t.Errorf("Debit = %d, want 5000", bal)
	}
	if len(store.debited) != 1 || store.debited[0] != "KTS-1-VPN" {
		t.Errorf("debited = %v, want [KTS-1-VPN]", store.debited)
	}
}

func TestDebit_GivenInsufficient_ThenPropagates(t *testing.T) {
	svc := New(&fakeStore{err: postgres.ErrInsufficientBalance})
	if _, err := svc.Debit(context.Background(), 1, domain.Money(1), "KTS-2-VPN"); !errors.Is(err, postgres.ErrInsufficientBalance) {
		t.Fatalf("Debit err = %v, want ErrInsufficientBalance", err)
	}
}

func TestCredit_GivenTopup_ThenNewBalanceAndOrderID(t *testing.T) {
	store := &fakeStore{balance: domain.Money(25000)}
	svc := New(store)

	bal, err := svc.Credit(context.Background(), 2, domain.Money(25000), "KTS-3-VPN")
	if err != nil {
		t.Fatalf("Credit: %v", err)
	}
	if bal != 25000 {
		t.Errorf("Credit = %d, want 25000", bal)
	}
	if len(store.credited) != 1 || store.credited[0] != "KTS-3-VPN" {
		t.Errorf("credited = %v, want [KTS-3-VPN]", store.credited)
	}
}

func TestSetBanned_GivenBanAndUnban_ThenCorrectStoreCalls(t *testing.T) {
	store := &fakeStore{}
	svc := New(store)

	if err := svc.SetBanned(context.Background(), 11, true); err != nil {
		t.Fatalf("SetBanned(true): %v", err)
	}
	if err := svc.SetBanned(context.Background(), 12, false); err != nil {
		t.Fatalf("SetBanned(false): %v", err)
	}
	if len(store.banned) != 1 || store.banned[0] != 11 {
		t.Errorf("banned = %v, want [11]", store.banned)
	}
	if len(store.unbanned) != 1 || store.unbanned[0] != 12 {
		t.Errorf("unbanned = %v, want [12]", store.unbanned)
	}
}

func TestListAndCount_GivenUsers_ThenDelegates(t *testing.T) {
	store := &fakeStore{list: []int64{1, 2, 3}, count: 3}
	svc := New(store)

	ids, err := svc.ListTelegramIDs(context.Background(), 100, 0)
	if err != nil {
		t.Fatalf("ListTelegramIDs: %v", err)
	}
	if len(ids) != 3 {
		t.Errorf("ids = %v, want 3 entries", ids)
	}
	n, err := svc.CountUsers(context.Background())
	if err != nil {
		t.Fatalf("CountUsers: %v", err)
	}
	if n != 3 {
		t.Errorf("count = %d, want 3", n)
	}
}

var _ Store = (*fakeStore)(nil)
