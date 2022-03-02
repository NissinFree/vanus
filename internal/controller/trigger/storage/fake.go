// Copyright 2022 Linkall Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package storage

import (
	"github.com/linkall-labs/vanus/internal/primitive"
)

type fake struct {
	subs map[string]*primitive.Subscription
}

func NewFakeStorage(config Config) (SubscriptionStorage, error) {
	s := &fake{
		subs: map[string]*primitive.Subscription{},
	}
	return s, nil
}

func (f *fake) Close() error {
	return nil
}

func (f *fake) CreateSubscription(sub *primitive.Subscription) error {
	f.subs[sub.ID] = sub
	return nil
}

func (f *fake) DeleteSubscription(id string) error {
	delete(f.subs, id)
	return nil
}

func (f *fake) GetSubscription(id string) (*primitive.Subscription, error) {
	return f.subs[id], nil
}

func (f *fake) ListSubscription() ([]*primitive.Subscription, error) {
	var list []*primitive.Subscription
	for _, sub := range f.subs {
		list = append(list, sub)
	}
	return list, nil
}
