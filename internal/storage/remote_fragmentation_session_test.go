package storage

import (
	"testing"
	"time"

	uuid "github.com/gofrs/uuid"
	"github.com/stretchr/testify/require"

	"github.com/brocaar/lora-app-server/internal/config"
	"github.com/brocaar/lora-app-server/internal/test"
	"github.com/brocaar/lorawan"
)

func (ts *StorageTestSuite) TestRemoteFragmentationSession() {
	assert := require.New(ts.T())

	nsClient := test.NewNetworkServerClient()
	config.C.NetworkServer.Pool = test.NewNetworkServerPool(nsClient)

	n := NetworkServer{
		Name:   "test",
		Server: "test:1234",
	}
	assert.NoError(CreateNetworkServer(ts.Tx(), &n))

	org := Organization{
		Name: "test-org",
	}
	assert.NoError(CreateOrganization(ts.Tx(), &org))

	sp := ServiceProfile{
		Name:            "test-sp",
		OrganizationID:  org.ID,
		NetworkServerID: n.ID,
	}
	assert.NoError(CreateServiceProfile(ts.Tx(), &sp))
	var spID uuid.UUID
	copy(spID[:], sp.ServiceProfile.Id)

	app := Application{
		Name:             "test-app",
		OrganizationID:   org.ID,
		ServiceProfileID: spID,
	}
	assert.NoError(CreateApplication(ts.Tx(), &app))

	dp := DeviceProfile{
		Name:            "test-dp",
		OrganizationID:  org.ID,
		NetworkServerID: n.ID,
	}
	assert.NoError(CreateDeviceProfile(ts.Tx(), &dp))
	var dpID uuid.UUID
	copy(dpID[:], dp.DeviceProfile.Id)

	d := Device{
		DevEUI:          lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
		ApplicationID:   app.ID,
		DeviceProfileID: dpID,
		Name:            "test-device",
		Description:     "test device",
	}
	assert.NoError(CreateDevice(ts.Tx(), &d))

	mg := MulticastGroup{
		Name:             "test-mg",
		MCAppSKey:        lorawan.AES128Key{1, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8},
		MCKey:            lorawan.AES128Key{1, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8},
		ServiceProfileID: spID,
	}
	assert.NoError(CreateMulticastGroup(ts.Tx(), &mg))
	var mgID uuid.UUID
	copy(mgID[:], mg.MulticastGroup.Id)

	rmg := RemoteMulticastSetup{
		DevEUI:           d.DevEUI,
		MulticastGroupID: mgID,
		McGroupID:        2,
		State:            RemoteMulticastSetupSetup,
	}
	assert.NoError(CreateRemoteMulticastSetup(ts.Tx(), &rmg))

	now := time.Now().Round(time.Second).UTC().Add(-time.Second)

	ts.T().Run("Create", func(t *testing.T) {
		assert := require.New(t)

		rfs := RemoteFragmentationSession{
			DevEUI:              d.DevEUI,
			FragIndex:           1,
			MCGroupIDs:          []int{rmg.McGroupID},
			NbFrag:              128,
			FragSize:            10,
			FragmentationMatrix: 5, // 101
			BlockAckDelay:       5,
			Padding:             3,
			Descriptor:          [4]byte{1, 2, 3, 4},
			State:               RemoteMulticastSetupSetup,
			RetryAfter:          now,
			RetryCount:          1,
		}
		assert.NoError(CreateRemoteFragmentationSession(ts.Tx(), &rfs))
		rfs.CreatedAt = rfs.CreatedAt.UTC().Round(time.Millisecond)
		rfs.UpdatedAt = rfs.UpdatedAt.UTC().Round(time.Millisecond)

		t.Run("Get", func(t *testing.T) {
			assert := require.New(t)

			rfsGet, err := GetRemoteFragmentationSession(ts.Tx(), d.DevEUI, rfs.FragIndex, false)
			assert.NoError(err)
			rfsGet.CreatedAt = rfsGet.CreatedAt.UTC().Round(time.Millisecond)
			rfsGet.UpdatedAt = rfsGet.UpdatedAt.UTC().Round(time.Millisecond)
			rfsGet.RetryAfter = rfsGet.RetryAfter.UTC()
			assert.Equal(rfs, rfsGet)
		})

		t.Run("GetPending no setup", func(t *testing.T) {
			assert := require.New(t)

			items, err := GetPendingRemoteFragmentationSessions(ts.Tx(), 10, 2)
			assert.NoError(err)
			assert.Len(items, 0)
		})

		t.Run("GetPending unicast", func(t *testing.T) {
			assert := require.New(t)
			rfs.MCGroupIDs = []int{}
			assert.NoError(UpdateRemoteFragmentationSession(ts.Tx(), &rfs))

			items, err := GetPendingRemoteFragmentationSessions(ts.Tx(), 10, 2)
			assert.NoError(err)
			assert.Len(items, 1)

			rfs.MCGroupIDs = []int{rmg.McGroupID}
			assert.NoError(UpdateRemoteFragmentationSession(ts.Tx(), &rfs))
		})

		t.Run("GetPending", func(t *testing.T) {
			assert := require.New(t)

			rmg.StateProvisioned = true
			assert.NoError(UpdateRemoteMulticastSetup(ts.Tx(), &rmg))

			items, err := GetPendingRemoteFragmentationSessions(ts.Tx(), 10, 2)
			assert.NoError(err)
			assert.Len(items, 1)

			// start a new transaction and make sure that we do not get the
			// locked items in the result-set.
			newTX, err := ts.DB().Beginx()
			assert.NoError(err)

			items, err = GetPendingRemoteFragmentationSessions(newTX, 10, 2)
			assert.NoError(err)
			assert.Len(items, 0)

			assert.NoError(newTX.Rollback())
		})

		t.Run("Update", func(t *testing.T) {
			assert := require.New(t)
			now = now.Add(time.Second)

			rfs.MCGroupIDs = []int{1, 2, 3}
			rfs.NbFrag = 64
			rfs.FragSize = 20
			rfs.FragmentationMatrix = 3
			rfs.BlockAckDelay = 10
			rfs.Padding = 6
			rfs.Descriptor = [4]byte{4, 3, 2, 1}
			rfs.State = RemoteMulticastSetupDelete
			rfs.StateProvisioned = true
			rfs.RetryAfter = now
			rfs.RetryCount = 2

			assert.NoError(UpdateRemoteFragmentationSession(ts.Tx(), &rfs))
			rfs.UpdatedAt = rfs.UpdatedAt.UTC().Round(time.Millisecond)

			rfsGet, err := GetRemoteFragmentationSession(ts.Tx(), d.DevEUI, rfs.FragIndex, false)
			assert.NoError(err)
			rfsGet.CreatedAt = rfsGet.CreatedAt.UTC().Round(time.Millisecond)
			rfsGet.UpdatedAt = rfsGet.UpdatedAt.UTC().Round(time.Millisecond)
			rfsGet.RetryAfter = rfsGet.RetryAfter.UTC()
			assert.Equal(rfs, rfsGet)

			t.Run("GetPending", func(t *testing.T) {
				assert := require.New(t)
				items, err := GetPendingRemoteFragmentationSessions(ts.Tx(), 10, 2)
				assert.NoError(err)
				assert.Len(items, 0)
			})

			t.Run("Delete", func(t *testing.T) {
				assert := require.New(t)

				assert.NoError(DeleteRemoteFragmentationSession(ts.Tx(), d.DevEUI, rfs.FragIndex))
				_, err := GetRemoteFragmentationSession(ts.Tx(), d.DevEUI, rfs.FragIndex, false)
				assert.Equal(ErrDoesNotExist, err)
			})
		})
	})
}
