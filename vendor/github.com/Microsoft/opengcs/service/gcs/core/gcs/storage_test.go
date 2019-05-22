package gcs

import (
	"fmt"
	"math/rand"
	"os"

	"github.com/Microsoft/opengcs/service/gcs/runtime/runc"
	"github.com/Microsoft/opengcs/service/gcs/transport"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Storage", func() {
	var (
		coreint *gcsCore
	)

	BeforeEach(func() {
		rtime, err := runc.NewRuntime("/tmp/gcs")
		Expect(err).NotTo(HaveOccurred())
		cint := NewGCSCore("/tmp/gcs", "/tmp", rtime, &transport.MockTransport{})
		coreint = cint.(*gcsCore)
	})

	Describe("getting the container paths", func() {
		var (
			validIndex uint32
		)
		BeforeEach(func() {
			validIndex = rand.Uint32()
		})

		Describe("getting the container storage path", func() {
			Context("when the index is a valid location", func() {
				It("should return the correct path", func() {
					Expect(coreint.getContainerStoragePath(validIndex)).To(Equal(fmt.Sprintf("/tmp/%d", validIndex)))
				})
			})
		})

		Describe("getting the unioning paths", func() {
			Context("when the index is a valid location", func() {
				It("should return the correct paths", func() {
					layerPrefix, scratchPath, upperdirPath, workdirPath, rootfsPath := coreint.getUnioningPaths(validIndex)
					Expect(layerPrefix).To(Equal(fmt.Sprintf("/tmp/%d", validIndex)))
					Expect(scratchPath).To(Equal(fmt.Sprintf("/tmp/%d/scratch", validIndex)))
					Expect(upperdirPath).To(Equal(fmt.Sprintf("/tmp/%d/scratch/upper", validIndex)))
					Expect(workdirPath).To(Equal(fmt.Sprintf("/tmp/%d/scratch/work", validIndex)))
					Expect(rootfsPath).To(Equal(fmt.Sprintf("/tmp/%d/rootfs", validIndex)))
				})
			})
		})

		Describe("getting the config path", func() {
			Context("when the ID is a valid string", func() {
				It("should return the correct path", func() {
					Expect(coreint.getConfigPath(validIndex)).To(Equal(fmt.Sprintf("/tmp/%d/config.json", validIndex)))
				})
			})
		})
	})

	Describe("checking if a path exists", func() {
		var (
			dirToTest  string
			fileToTest string
			path       string
			exists     bool
			err        error
		)
		BeforeEach(func() {
			dirToTest = "/tmp/testdir"
			fileToTest = "/tmp/testfile"
		})
		JustBeforeEach(func() {
			_, err = os.Stat(path)
			if err != nil {
				if os.IsNotExist(err) {
					// Old code did this
					err = nil
				}
			} else {
				exists = true
			}
		})
		AssertDoesNotExist := func() {
			It("should not report exists", func() {
				Expect(exists).To(BeFalse())
			})
			It("should not return an error", func() {
				Expect(err).NotTo(HaveOccurred())
			})
		}
		AssertExists := func() {
			It("should report exists", func() {
				Expect(exists).To(BeTrue())
			})
			It("should not return an error", func() {
				Expect(err).NotTo(HaveOccurred())
			})
		}
		Context("the paths don't exist", func() {
			Context("using the directory path", func() {
				BeforeEach(func() {
					path = dirToTest
				})
				AssertDoesNotExist()
			})
			Context("using the file path", func() {
				BeforeEach(func() {
					path = fileToTest
				})
				AssertDoesNotExist()
			})
		})
		Context("the paths exist", func() {
			BeforeEach(func() {
				err := os.Mkdir(dirToTest, 0600)
				Expect(err).NotTo(HaveOccurred())
				_, err = os.OpenFile(fileToTest, os.O_RDONLY|os.O_CREATE, 0600)
				Expect(err).NotTo(HaveOccurred())
			})
			AfterEach(func() {
				err := os.Remove(dirToTest)
				Expect(err).NotTo(HaveOccurred())
				err = os.Remove(fileToTest)
				Expect(err).NotTo(HaveOccurred())
			})
			Context("using the directory path", func() {
				BeforeEach(func() {
					path = dirToTest
				})
				AssertExists()
			})
			Context("using the file path", func() {
				BeforeEach(func() {
					path = fileToTest
				})
				AssertExists()
			})
		})
	})
})
