// Code generated by MockGen. DO NOT EDIT.
// Source: client.go

// Package authorization is a generated GoMock package.
package authorization

import (
	context "context"
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	vanus "github.com/vanus-labs/vanus/internal/primitive/vanus"
)

// MockRoleClient is a mock of RoleClient interface.
type MockRoleClient struct {
	ctrl     *gomock.Controller
	recorder *MockRoleClientMockRecorder
}

// MockRoleClientMockRecorder is the mock recorder for MockRoleClient.
type MockRoleClientMockRecorder struct {
	mock *MockRoleClient
}

// NewMockRoleClient creates a new mock instance.
func NewMockRoleClient(ctrl *gomock.Controller) *MockRoleClient {
	mock := &MockRoleClient{ctrl: ctrl}
	mock.recorder = &MockRoleClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockRoleClient) EXPECT() *MockRoleClientMockRecorder {
	return m.recorder
}

// GetUserEventbusID mocks base method.
func (m *MockRoleClient) GetUserEventbusID(ctx context.Context, user string) (vanus.IDList, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetUserEventbusID", ctx, user)
	ret0, _ := ret[0].(vanus.IDList)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetUserEventbusID indicates an expected call of GetUserEventbusID.
func (mr *MockRoleClientMockRecorder) GetUserEventbusID(ctx, user interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetUserEventbusID", reflect.TypeOf((*MockRoleClient)(nil).GetUserEventbusID), ctx, user)
}

// GetUserNamespaceID mocks base method.
func (m *MockRoleClient) GetUserNamespaceID(ctx context.Context, user string) (vanus.IDList, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetUserNamespaceID", ctx, user)
	ret0, _ := ret[0].(vanus.IDList)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetUserNamespaceID indicates an expected call of GetUserNamespaceID.
func (mr *MockRoleClientMockRecorder) GetUserNamespaceID(ctx, user interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetUserNamespaceID", reflect.TypeOf((*MockRoleClient)(nil).GetUserNamespaceID), ctx, user)
}

// GetUserRole mocks base method.
func (m *MockRoleClient) GetUserRole(ctx context.Context, user string) ([]*UserRole, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetUserRole", ctx, user)
	ret0, _ := ret[0].([]*UserRole)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetUserRole indicates an expected call of GetUserRole.
func (mr *MockRoleClientMockRecorder) GetUserRole(ctx, user interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetUserRole", reflect.TypeOf((*MockRoleClient)(nil).GetUserRole), ctx, user)
}

// GetUserSubscriptionID mocks base method.
func (m *MockRoleClient) GetUserSubscriptionID(ctx context.Context, user string) (vanus.IDList, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetUserSubscriptionID", ctx, user)
	ret0, _ := ret[0].(vanus.IDList)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetUserSubscriptionID indicates an expected call of GetUserSubscriptionID.
func (mr *MockRoleClientMockRecorder) GetUserSubscriptionID(ctx, user interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetUserSubscriptionID", reflect.TypeOf((*MockRoleClient)(nil).GetUserSubscriptionID), ctx, user)
}

// IsClusterAdmin mocks base method.
func (m *MockRoleClient) IsClusterAdmin(ctx context.Context, user string) (bool, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "IsClusterAdmin", ctx, user)
	ret0, _ := ret[0].(bool)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// IsClusterAdmin indicates an expected call of IsClusterAdmin.
func (mr *MockRoleClientMockRecorder) IsClusterAdmin(ctx, user interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "IsClusterAdmin", reflect.TypeOf((*MockRoleClient)(nil).IsClusterAdmin), ctx, user)
}
