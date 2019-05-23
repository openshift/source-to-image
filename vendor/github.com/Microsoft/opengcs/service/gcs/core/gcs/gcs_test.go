package gcs

import (
	"fmt"

	"github.com/Microsoft/opengcs/service/gcs/prot"
	"github.com/Microsoft/opengcs/service/gcs/runtime/mockruntime"
	"github.com/Microsoft/opengcs/service/gcs/stdio"
	"github.com/Microsoft/opengcs/service/gcs/transport"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	oci "github.com/opencontainers/runtime-spec/specs-go"
	"golang.org/x/sys/unix"
)

var _ = Describe("GCS", func() {
	var (
		err error
	)
	AssertNoError := func() {
		It("should not produce an error", func() {
			Expect(err).NotTo(HaveOccurred())
		})
	}
	AssertError := func() {
		It("should produce an error", func() {
			Expect(err).To(HaveOccurred())
		})
	}
	Describe("unittests", func() {
		Describe("calling processParametersToOCI", func() {
			var (
				params  prot.ProcessParameters
				process *oci.Process
			)
			JustBeforeEach(func() {
				process, err = processParametersToOCI(params)
			})
			Context("params are zeroed", func() {
				BeforeEach(func() {
					params = prot.ProcessParameters{}
				})
				AssertNoError()
				It("should output an oci.Process with non-defaulted fields zeroed", func() {
					Expect(*process).To(Equal(oci.Process{
						Args: []string{},
						Env:  []string{},
						User: oci.User{UID: 0, GID: 0},
						Capabilities: &oci.LinuxCapabilities{
							Bounding: []string{
								"CAP_AUDIT_WRITE",
								"CAP_KILL",
								"CAP_NET_BIND_SERVICE",
								"CAP_SYS_ADMIN",
								"CAP_NET_ADMIN",
								"CAP_SETGID",
								"CAP_SETUID",
								"CAP_CHOWN",
								"CAP_FOWNER",
								"CAP_DAC_OVERRIDE",
								"CAP_NET_RAW",
							},
							Effective: []string{
								"CAP_AUDIT_WRITE",
								"CAP_KILL",
								"CAP_NET_BIND_SERVICE",
								"CAP_SYS_ADMIN",
								"CAP_NET_ADMIN",
								"CAP_SETGID",
								"CAP_SETUID",
								"CAP_CHOWN",
								"CAP_FOWNER",
								"CAP_DAC_OVERRIDE",
								"CAP_NET_RAW",
							},
							Inheritable: []string{
								"CAP_AUDIT_WRITE",
								"CAP_KILL",
								"CAP_NET_BIND_SERVICE",
								"CAP_SYS_ADMIN",
								"CAP_NET_ADMIN",
								"CAP_SETGID",
								"CAP_SETUID",
								"CAP_CHOWN",
								"CAP_FOWNER",
								"CAP_DAC_OVERRIDE",
								"CAP_NET_RAW",
							},
							Permitted: []string{
								"CAP_AUDIT_WRITE",
								"CAP_KILL",
								"CAP_NET_BIND_SERVICE",
								"CAP_SYS_ADMIN",
								"CAP_NET_ADMIN",
								"CAP_SETGID",
								"CAP_SETUID",
								"CAP_CHOWN",
								"CAP_FOWNER",
								"CAP_DAC_OVERRIDE",
								"CAP_NET_RAW",
							},
							Ambient: []string{
								"CAP_AUDIT_WRITE",
								"CAP_KILL",
								"CAP_NET_BIND_SERVICE",
								"CAP_SYS_ADMIN",
								"CAP_NET_ADMIN",
								"CAP_SETGID",
								"CAP_SETUID",
								"CAP_CHOWN",
								"CAP_FOWNER",
								"CAP_DAC_OVERRIDE",
								"CAP_NET_RAW",
							},
						},
						Rlimits: []oci.POSIXRlimit{
							oci.POSIXRlimit{Type: "RLIMIT_NOFILE", Hard: 1024, Soft: 1024},
						},
						NoNewPrivileges: true,
					}))
				})
			})
			Context("params are set to values", func() {
				BeforeEach(func() {
					params = prot.ProcessParameters{
						CommandArgs:      []string{"sh", "-c", "sleep", "20"},
						WorkingDirectory: "/home/user/work",
						Environment: map[string]string{
							"PATH": "/this/is/my/path",
						},
						EmulateConsole:   true,
						CreateStdInPipe:  true,
						CreateStdOutPipe: true,
						CreateStdErrPipe: true,
						IsExternal:       true,
					}
				})
				AssertNoError()
				It("should output an oci.Process which matches the input values", func() {
					Expect(*process).To(Equal(oci.Process{
						Args:     []string{"sh", "-c", "sleep", "20"},
						Cwd:      "/home/user/work",
						Env:      []string{"PATH=/this/is/my/path"},
						Terminal: true,

						User: oci.User{UID: 0, GID: 0},
						Capabilities: &oci.LinuxCapabilities{
							Bounding: []string{
								"CAP_AUDIT_WRITE",
								"CAP_KILL",
								"CAP_NET_BIND_SERVICE",
								"CAP_SYS_ADMIN",
								"CAP_NET_ADMIN",
								"CAP_SETGID",
								"CAP_SETUID",
								"CAP_CHOWN",
								"CAP_FOWNER",
								"CAP_DAC_OVERRIDE",
								"CAP_NET_RAW",
							},
							Effective: []string{
								"CAP_AUDIT_WRITE",
								"CAP_KILL",
								"CAP_NET_BIND_SERVICE",
								"CAP_SYS_ADMIN",
								"CAP_NET_ADMIN",
								"CAP_SETGID",
								"CAP_SETUID",
								"CAP_CHOWN",
								"CAP_FOWNER",
								"CAP_DAC_OVERRIDE",
								"CAP_NET_RAW",
							},
							Inheritable: []string{
								"CAP_AUDIT_WRITE",
								"CAP_KILL",
								"CAP_NET_BIND_SERVICE",
								"CAP_SYS_ADMIN",
								"CAP_NET_ADMIN",
								"CAP_SETGID",
								"CAP_SETUID",
								"CAP_CHOWN",
								"CAP_FOWNER",
								"CAP_DAC_OVERRIDE",
								"CAP_NET_RAW",
							},
							Permitted: []string{
								"CAP_AUDIT_WRITE",
								"CAP_KILL",
								"CAP_NET_BIND_SERVICE",
								"CAP_SYS_ADMIN",
								"CAP_NET_ADMIN",
								"CAP_SETGID",
								"CAP_SETUID",
								"CAP_CHOWN",
								"CAP_FOWNER",
								"CAP_DAC_OVERRIDE",
								"CAP_NET_RAW",
							},
							Ambient: []string{
								"CAP_AUDIT_WRITE",
								"CAP_KILL",
								"CAP_NET_BIND_SERVICE",
								"CAP_SYS_ADMIN",
								"CAP_NET_ADMIN",
								"CAP_SETGID",
								"CAP_SETUID",
								"CAP_CHOWN",
								"CAP_FOWNER",
								"CAP_DAC_OVERRIDE",
								"CAP_NET_RAW",
							},
						},
						Rlimits: []oci.POSIXRlimit{
							oci.POSIXRlimit{Type: "RLIMIT_NOFILE", Hard: 1024, Soft: 1024},
						},
						NoNewPrivileges: true,
					}))
				})
			})
			Context("CommandLine is used rather than CommandArgs", func() {
				BeforeEach(func() {
					params = prot.ProcessParameters{
						CommandLine:      "sh -c sleep 20",
						WorkingDirectory: "/home/user/work",
						Environment: map[string]string{
							"PATH": "/this/is/my/path",
						},
						EmulateConsole:   true,
						CreateStdInPipe:  true,
						CreateStdOutPipe: true,
						CreateStdErrPipe: true,
						IsExternal:       true,
					}
				})
				AssertNoError()
				It("should output an oci.Process which matches the input values", func() {
					Expect(*process).To(Equal(oci.Process{
						Args:     []string{"sh", "-c", "sleep", "20"},
						Cwd:      "/home/user/work",
						Env:      []string{"PATH=/this/is/my/path"},
						Terminal: true,

						User: oci.User{UID: 0, GID: 0},
						Capabilities: &oci.LinuxCapabilities{
							Bounding: []string{
								"CAP_AUDIT_WRITE",
								"CAP_KILL",
								"CAP_NET_BIND_SERVICE",
								"CAP_SYS_ADMIN",
								"CAP_NET_ADMIN",
								"CAP_SETGID",
								"CAP_SETUID",
								"CAP_CHOWN",
								"CAP_FOWNER",
								"CAP_DAC_OVERRIDE",
								"CAP_NET_RAW",
							},
							Effective: []string{
								"CAP_AUDIT_WRITE",
								"CAP_KILL",
								"CAP_NET_BIND_SERVICE",
								"CAP_SYS_ADMIN",
								"CAP_NET_ADMIN",
								"CAP_SETGID",
								"CAP_SETUID",
								"CAP_CHOWN",
								"CAP_FOWNER",
								"CAP_DAC_OVERRIDE",
								"CAP_NET_RAW",
							},
							Inheritable: []string{
								"CAP_AUDIT_WRITE",
								"CAP_KILL",
								"CAP_NET_BIND_SERVICE",
								"CAP_SYS_ADMIN",
								"CAP_NET_ADMIN",
								"CAP_SETGID",
								"CAP_SETUID",
								"CAP_CHOWN",
								"CAP_FOWNER",
								"CAP_DAC_OVERRIDE",
								"CAP_NET_RAW",
							},
							Permitted: []string{
								"CAP_AUDIT_WRITE",
								"CAP_KILL",
								"CAP_NET_BIND_SERVICE",
								"CAP_SYS_ADMIN",
								"CAP_NET_ADMIN",
								"CAP_SETGID",
								"CAP_SETUID",
								"CAP_CHOWN",
								"CAP_FOWNER",
								"CAP_DAC_OVERRIDE",
								"CAP_NET_RAW",
							},
							Ambient: []string{
								"CAP_AUDIT_WRITE",
								"CAP_KILL",
								"CAP_NET_BIND_SERVICE",
								"CAP_SYS_ADMIN",
								"CAP_NET_ADMIN",
								"CAP_SETGID",
								"CAP_SETUID",
								"CAP_CHOWN",
								"CAP_FOWNER",
								"CAP_DAC_OVERRIDE",
								"CAP_NET_RAW",
							},
						},
						Rlimits: []oci.POSIXRlimit{
							oci.POSIXRlimit{Type: "RLIMIT_NOFILE", Hard: 1024, Soft: 1024},
						},
						NoNewPrivileges: true,
					}))
				})
			})
		})

		Describe("calling processParamCommandLineToOCIArgs", func() {
			var (
				commandLine string
				args        []string
			)
			JustBeforeEach(func() {
				args, err = processParamCommandLineToOCIArgs(commandLine)
			})
			Context("commandLine is empty", func() {
				BeforeEach(func() {
					commandLine = ""
				})
				AssertNoError()
				It("should produce an empty slice", func() {
					Expect(args).To(BeEmpty())
				})
			})
			Context("commandLine has one argument", func() {
				BeforeEach(func() {
					commandLine = "sh"
				})
				AssertNoError()
				It("should produce a slice with just that argument", func() {
					Expect(args).To(Equal([]string{"sh"}))
				})
			})
			Context("commandLine has two arguments", func() {
				BeforeEach(func() {
					commandLine = "sleep 100"
				})
				AssertNoError()
				It("should produce a slice with both arguments", func() {
					Expect(args).To(Equal([]string{"sleep", "100"}))
				})
			})
			Context("commandLine has many arguments", func() {
				BeforeEach(func() {
					commandLine = "sh -c cat /bin/ls と℅Eṁに"
				})
				AssertNoError()
				It("should produce a slice with all the arguments", func() {
					Expect(args).To(Equal([]string{"sh", "-c", "cat", "/bin/ls", "と℅Eṁに"}))
				})
			})
			for _, quoteType := range []string{"\"", "'"} {
				Context(fmt.Sprintf("using quote type %s", quoteType), func() {
					Context("commandLine has a single quoted string", func() {
						BeforeEach(func() {
							commandLine = fmt.Sprintf("%ssh%s", quoteType, quoteType)
						})
						AssertNoError()
						It("should produce a slice without the quotes", func() {
							Expect(args).To(Equal([]string{"sh"}))
						})
					})
					Context("commandLine has a single quoted string as second argument", func() {
						BeforeEach(func() {
							commandLine = fmt.Sprintf("sh %sa%s", quoteType, quoteType)
						})
						AssertNoError()
						It("should produce a slice without the quotes", func() {
							Expect(args).To(Equal([]string{"sh", "a"}))
						})
					})
					Context("commandLine has a single quoted string with spaces in it", func() {
						BeforeEach(func() {
							commandLine = fmt.Sprintf("%sa b c%s", quoteType, quoteType)
						})
						AssertNoError()
						It("should produce a one-element slice without the quotes", func() {
							Expect(args).To(Equal([]string{"a b c"}))
						})
					})
					Context("commandLine has multiple quoted strings with spaces in them", func() {
						BeforeEach(func() {
							commandLine = fmt.Sprintf("%sa b c%s %sd e%s", quoteType, quoteType, quoteType, quoteType)
						})
						AssertNoError()
						It("should produce a multi-element slice without the quotes", func() {
							Expect(args).To(Equal([]string{"a b c", "d e"}))
						})
					})
					Context("commandLine has only spaces in quoted initial argument", func() {
						BeforeEach(func() {
							commandLine = fmt.Sprintf("%s  %s", quoteType, quoteType)
						})
						AssertNoError()
						It("should remove the argument", func() {
							Expect(args).To(BeEmpty())
						})
					})
					Context("commandLine has quoted arguments with only spaces as the non-initial argument", func() {
						BeforeEach(func() {
							commandLine = fmt.Sprintf("sh %s   %s", quoteType, quoteType)
						})
						AssertNoError()
						It("should remove the argument", func() {
							Expect(args).To(Equal([]string{"sh"}))
						})
					})
					Context("commandLine has spaces and non-spaces in a quoted argument", func() {
						BeforeEach(func() {
							commandLine = fmt.Sprintf("%s sh  %s", quoteType, quoteType)
						})
						AssertNoError()
						It("should preserve the spaces and the argument", func() {
							Expect(args).To(Equal([]string{" sh  "}))
						})
					})
					Context("commandLine has unclosed quotes at the beginning of string", func() {
						BeforeEach(func() {
							commandLine = fmt.Sprintf("%s sh", quoteType)
						})
						AssertError()
					})
					Context("commandLine has unclosed quotes at the end of string", func() {
						BeforeEach(func() {
							commandLine = fmt.Sprintf("sh %s", quoteType)
						})
						AssertError()
					})
					Context("commandLine has unclosed quotes with characters after them", func() {
						BeforeEach(func() {
							commandLine = fmt.Sprintf("sh %s  o ", quoteType)
						})
						AssertError()
					})
					Context("commandLine has unclosed quotes along-side closed quotes", func() {
						BeforeEach(func() {
							commandLine = fmt.Sprintf("sh %s -c %s %s o ", quoteType, quoteType, quoteType)
						})
						AssertError()
					})
				})
			}
			Context("commandLine has multiple quoted strings with escaped quotes in them", func() {
				BeforeEach(func() {
					commandLine = "\"a b \\\" c\" \"d e \\\"\""
				})
				AssertNoError()
				It("should produce a multi-element slice with only the escaped quotes", func() {
					Expect(args).To(Equal([]string{"a b \" c", "d e \""}))
				})
			})
			Context("commandLine has spaces at the edges", func() {
				BeforeEach(func() {
					commandLine = " sh "
				})
				AssertNoError()
				It("should produce a one-element slice without the spaces", func() {
					Expect(args).To(Equal([]string{"sh"}))
				})
			})
			Context("commandLine has multiple spaces at the edges", func() {
				BeforeEach(func() {
					commandLine = "  sh     "
				})
				AssertNoError()
				It("should produce a one-element slice without the spaces", func() {
					Expect(args).To(Equal([]string{"sh"}))
				})
			})
			Context("commandLine has multiple spaces in between arguments", func() {
				BeforeEach(func() {
					commandLine = "   sh   -c  ls  "
				})
				AssertNoError()
				It("should produce a multi-element slice without extra spaces", func() {
					Expect(args).To(Equal([]string{"sh", "-c", "ls"}))
				})
			})
			Context("commandLine has a combination of previous test contexts", func() {
				BeforeEach(func() {
					commandLine = " \"sh  \" -c \"  ls    \\\"/bin\\\" \"   "
				})
				AssertNoError()
				It("should parse the string correctly", func() {
					Expect(args).To(Equal([]string{"sh  ", "-c", "  ls    \"/bin\" "}))
				})
			})
		})

		Describe("calling processParamEnvToOCIEnv", func() {
			var (
				environment     map[string]string
				environmentList []string
			)
			JustBeforeEach(func() {
				environmentList = processParamEnvToOCIEnv(environment)
			})
			Context("environment is empty", func() {
				BeforeEach(func() {
					environment = make(map[string]string)
				})
				It("should produce an empty list", func() {
					Expect(environmentList).To(BeEmpty())
				})
			})
			Context("environment has one element", func() {
				BeforeEach(func() {
					environment = make(map[string]string)
					environment["TEST"] = "this is a test variable!"
				})
				It("should produce a list containing that element", func() {
					Expect(environmentList).To(Equal([]string{"TEST=this is a test variable!"}))
				})
			})
			Context("environment has two elements", func() {
				BeforeEach(func() {
					environment = make(map[string]string)
					environment["TEST"] = "this is a test variable!"
					environment["PATH"] = "/this/is/a/test/path"
				})
				It("should produce a list containing both elements", func() {
					Expect(environmentList).To(ConsistOf([]string{
						"TEST=this is a test variable!",
						"PATH=/this/is/a/test/path",
					}))
				})
			})
			Context("environment has many elements", func() {
				BeforeEach(func() {
					environment = make(map[string]string)
					environment["TEST"] = "this is a test variable!"
					environment["PATH"] = "/this/is/a/test/path"
					environment["HELLO"] = "world"
					environment["VaR"] = "variable"
					environment["¥¢£"] = "ピめと"
				})
				It("should produce a list containing all the elements", func() {
					Expect(environmentList).To(ConsistOf([]string{
						"TEST=this is a test variable!",
						"PATH=/this/is/a/test/path",
						"HELLO=world",
						"VaR=variable",
						"¥¢£=ピめと",
					}))
				})
			})
		})

		Describe("calling into the primary GCS functions", func() {
			var (
				coreint                       *gcsCore
				containerID                   string
				processID                     int
				initialExecParams             prot.ProcessParameters
				nonInitialExecParams          prot.ProcessParameters
				externalParams                prot.ProcessParameters
				fullStdioSet                  stdio.ConnectionSettings
				mappedVirtualDisk             prot.MappedVirtualDisk
				mappedDirectory               prot.MappedDirectory
				diskModificationRequest       prot.ResourceModificationRequestResponse
				diskModificationRequestRemove prot.ResourceModificationRequestResponse
				dirModificationRequest        prot.ResourceModificationRequestResponse
				dirModificationRequestRemove  prot.ResourceModificationRequestResponse
				err                           error
			)
			BeforeEach(func() {
				rtime := mockruntime.NewRuntime("/tmp/gcs")
				cint := NewGCSCore("/tmp/gcs", "/tmp", rtime, &transport.MockTransport{})
				coreint = cint.(*gcsCore)
				containerID = "01234567-89ab-cdef-0123-456789abcdef"
				processID = 101
				initialExecParams = prot.ProcessParameters{
					CreateStdInPipe:  true,
					CreateStdOutPipe: true,
					CreateStdErrPipe: true,
					IsExternal:       false,
					OCISpecification: &oci.Spec{},
				}
				nonInitialExecParams = prot.ProcessParameters{
					CommandLine:      "cat file",
					WorkingDirectory: "/",
					Environment:      map[string]string{"PATH": "/usr/bin:/usr/sbin"},
					EmulateConsole:   true,
					CreateStdInPipe:  true,
					CreateStdOutPipe: true,
					CreateStdErrPipe: true,
					IsExternal:       false,
				}
				externalParams = prot.ProcessParameters{
					CommandLine:      "cat file",
					WorkingDirectory: "/",
					Environment:      map[string]string{"PATH": "/usr/bin:/usr/sbin"},
					EmulateConsole:   true,
					CreateStdInPipe:  true,
					CreateStdOutPipe: true,
					CreateStdErrPipe: true,
					IsExternal:       true,
					OCISpecification: &oci.Spec{},
				}
				var in, out, err uint32 = 0, 1, 2
				fullStdioSet = stdio.ConnectionSettings{
					StdIn:  &in,
					StdOut: &out,
					StdErr: &err,
				}

				mappedVirtualDisk = prot.MappedVirtualDisk{
					ContainerPath:     "/path/inside/container",
					Lun:               5,
					CreateInUtilityVM: true,
					ReadOnly:          false,
				}
				mappedDirectory = prot.MappedDirectory{
					ContainerPath:     "abcdefghijklmnopqrstuvwxyz",
					CreateInUtilityVM: true,
					ReadOnly:          false,
					Port:              5,
				}

				diskModificationRequest = prot.ResourceModificationRequestResponse{
					ResourceType: prot.PtMappedVirtualDisk,
					RequestType:  prot.RtAdd,
					Settings:     &mappedVirtualDisk,
				}
				diskModificationRequestRemove = prot.ResourceModificationRequestResponse{
					ResourceType: prot.PtMappedVirtualDisk,
					RequestType:  prot.RtRemove,
					Settings:     &mappedVirtualDisk,
				}
				dirModificationRequest = prot.ResourceModificationRequestResponse{
					ResourceType: prot.PtMappedDirectory,
					RequestType:  prot.RtAdd,
					Settings:     &mappedDirectory,
				}
				dirModificationRequestRemove = prot.ResourceModificationRequestResponse{
					ResourceType: prot.PtMappedDirectory,
					RequestType:  prot.RtRemove,
					Settings:     &mappedDirectory,
				}
			})
			Describe("calling ExecProcess", func() {
				var (
					params      prot.ProcessParameters
					errDoneChan chan<- struct{}
				)
				JustBeforeEach(func() {
					_, errDoneChan, err = coreint.ExecProcess(containerID, params, fullStdioSet)
				})
				AfterEach(func() {
					if errDoneChan != nil {
						errDoneChan <- struct{}{}
					}
				})
				Context("it is the initial process", func() {
					BeforeEach(func() {
						params = initialExecParams
					})
					Context("the container has not already been created", func() {
						It("should produce an error", func() {
							Expect(err).To(HaveOccurred())
						})
					})
				})
				Context("it is not the initial process", func() {
					BeforeEach(func() {
						params = nonInitialExecParams
					})
					Context("the container has not already been created", func() {
						It("should produce an error", func() {
							Expect(err).To(HaveOccurred())
						})
					})
				})
			})
			Describe("calling SignalContainer", func() {
				Context("using signal SIGKILL", func() {
					JustBeforeEach(func() {
						err = coreint.SignalContainer(containerID, unix.SIGKILL)
					})
					Context("the container has not already been created", func() {
						It("should produce an error", func() {
							Expect(err).To(HaveOccurred())
						})
					})
				})
				Context("using signal SIGTERM", func() {
					JustBeforeEach(func() {
						err = coreint.SignalContainer(containerID, unix.SIGTERM)
					})
					Context("the container has not already been created", func() {
						It("should produce an error", func() {
							Expect(err).To(HaveOccurred())
						})
					})
				})
			})
			Describe("calling SignalProcess", func() {
				var (
					sigkillOptions prot.SignalProcessOptions
				)
				BeforeEach(func() {
					sigkillOptions = prot.SignalProcessOptions{Signal: int32(unix.SIGKILL)}
				})
				JustBeforeEach(func() {
					err = coreint.SignalProcess(processID, sigkillOptions)
				})
				Context("the process has not already been created", func() {
					It("should produce an error", func() {
						Expect(err).To(HaveOccurred())
					})
				})
			})
			Describe("calling GetProperties", func() {
				var (
					query string
				)
				JustBeforeEach(func() {
					_, err = coreint.GetProperties(containerID, query)
				})
				Context("the container has not already been created", func() {
					It("should produce an error", func() {
						Expect(err).To(HaveOccurred())
					})
				})
			})
			Describe("calling RunExternalProcess", func() {
				JustBeforeEach(func() {
					_, err = coreint.RunExternalProcess(externalParams, fullStdioSet)
				})
				It("should not produce an error", func() {
					Expect(err).NotTo(HaveOccurred())
				})
			})
			Describe("calling ModifySettings", func() {
				Context("adding a mapped virtual disk", func() {
					Context("the lun is not already in use", func() {
						JustBeforeEach(func() {
							err = coreint.ModifySettings(containerID, &diskModificationRequest)
						})
						Context("the container has not already been created", func() {
							It("should produce an error", func() {
								Expect(err).To(HaveOccurred())
							})
						})
					})
				})
				Context("removing a mapped virtual disk", func() {
					Context("the disk has been added", func() {
						JustBeforeEach(func() {
							err = coreint.ModifySettings(containerID, &diskModificationRequestRemove)
						})
						Context("the container has not already been created", func() {
							It("should produce an error", func() {
								Expect(err).To(HaveOccurred())
							})
						})
					})
				})
				Context("adding a mapped directory", func() {
					Context("the port is not already in use", func() {
						JustBeforeEach(func() {
							err = coreint.ModifySettings(containerID, &dirModificationRequest)
						})
						Context("the container has not already been created", func() {
							It("should produce an error", func() {
								Expect(err).To(HaveOccurred())
							})
						})
					})
				})
				Context("removing a mapped directory", func() {
					Context("the directory has been added", func() {
						JustBeforeEach(func() {
							err = coreint.ModifySettings(containerID, &dirModificationRequestRemove)
						})
						Context("the container has not already been created", func() {
							It("should produce an error", func() {
								Expect(err).To(HaveOccurred())
							})
						})
					})
				})
			})
			Describe("calling wait container", func() {
				JustBeforeEach(func() {
					var waitFn func() prot.NotificationType
					waitFn, err = coreint.WaitContainer(containerID)
					if err == nil {
						waitFn()
					}
				})
				Context("container does not exist", func() {
					It("should produce errors", func() {
						Expect(err).To(HaveOccurred())
					})
				})
			})
			Describe("calling wait process", func() {
				var (
					pid int
				)
				JustBeforeEach(func() {
					var exitCodeChan <-chan int
					var doneChan chan<- bool
					exitCodeChan, doneChan, err = coreint.WaitProcess(pid)
					if err == nil {
						<-exitCodeChan
						doneChan <- true
					}
				})
				Context("process does not exist", func() {
					It("should produce an error", func() {
						Expect(err).To(HaveOccurred())
					})
				})
				Context("process does exist", func() {
					JustBeforeEach(func() {
						pid, err = coreint.RunExternalProcess(externalParams, fullStdioSet)
						Expect(err).NotTo(HaveOccurred())
					})
					It("should not produce an error", func() {
						Expect(err).NotTo(HaveOccurred())
					})
				})
			})
		})
	})
})
