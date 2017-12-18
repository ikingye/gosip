package transaction_test

import (
	"fmt"
	"sync"
	"time"

	"github.com/ghettovoice/gosip/core"
	"github.com/ghettovoice/gosip/testutils"
	"github.com/ghettovoice/gosip/transaction"
	"github.com/ghettovoice/gossip/base"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ServerTx", func() {
	var (
		tpl *testutils.MockTransportLayer
		txl transaction.Layer
	)

	//serverAddr := "localhost:8001"
	clientAddr := "localhost:9001"

	BeforeEach(func() {
		tpl = testutils.NewMockTransportLayer()
		txl = transaction.NewLayer(tpl)
	})
	AfterEach(func(done Done) {
		txl.Cancel()
		<-txl.Done()
		close(done)
	}, 3)

	Context("just initialized", func() {
		It("should has transport layer", func() {
			Expect(txl.Transport()).To(Equal(tpl))
		})
	})
	// TODO: think about how to test Tx state switches and deletion
	Context("when INVITE request arrives", func() {
		var inviteTxKey, ackTxKey transaction.TxKey
		var err error
		var invite, trying, ok, notOk, ack, notOkAck core.Message
		var inviteBranch string

		BeforeEach(func() {
			inviteBranch = core.GenerateBranch()
			invite = request([]string{
				"INVITE sip:bob@example.com SIP/2.0",
				"Via: SIP/2.0/UDP " + clientAddr + ";branch=" + inviteBranch,
				"CSeq: 1 INVITE",
				"",
				"",
			})
			trying = response([]string{
				"SIP/2.0 100 Trying",
				"Via: SIP/2.0/UDP " + clientAddr + ";branch=" + inviteBranch,
				"CSeq: 1 INVITE",
				"",
				"",
			})
			ok = response([]string{
				"SIP/2.0 200 OK",
				"CSeq: 1 INVITE",
				"Via: SIP/2.0/UDP " + clientAddr + ";branch=" + inviteBranch,
				"",
				"",
			})
			notOk = response([]string{
				"SIP/2.0 400 Bad Request",
				"CSeq: 1 INVITE",
				"Via: SIP/2.0/UDP " + clientAddr + ";branch=" + inviteBranch,
				"",
				"",
			})
			ack = request([]string{
				"ACK sip:bob@example.com SIP/2.0",
				"Via: SIP/2.0/UDP " + clientAddr + ";branch=" + base.GenerateBranch(),
				"CSeq: 1 ACK",
				"",
				"",
			})
			notOkAck = request([]string{
				"ACK sip:bob@example.com SIP/2.0",
				"Via: SIP/2.0/UDP " + clientAddr + ";branch=" + inviteBranch,
				"CSeq: 1 ACK",
				"",
				"",
			})

			inviteTxKey, err = transaction.MakeServerTxKey(invite)
			Expect(err).ToNot(HaveOccurred())
			ackTxKey, err = transaction.MakeServerTxKey(ack)
			Expect(err).ToNot(HaveOccurred())

			go func() {
				By(fmt.Sprintf("UAC sends %s", invite.Short()))
				tpl.InMsgs <- invite
			}()
		})

		It("should open server tx and pass up TxMessage", func() {
			By(fmt.Sprintf("UAS receives %s", invite.Short()))
			msg := <-txl.Messages()
			Expect(msg).ToNot(BeNil())
			Expect(msg.String()).To(Equal(invite.String()))
			Expect(msg.Tx()).ToNot(BeNil())
			Expect(msg.Tx().Key()).To(Equal(inviteTxKey))
		})

		Context("when INVITE server tx created", func() {
			BeforeEach(func() {
				<-txl.Messages()
			})

			It("should send 100 Trying after Timer_1xx fired", func() {
				time.Sleep(transaction.Timer_1xx + time.Millisecond)
				By(fmt.Sprintf("UAC waits %s", trying.Short()))
				msg := <-tpl.OutMsgs
				Expect(msg).ToNot(BeNil())
				Expect(msg.String()).To(Equal(trying.String()))
			})

			It("should send in transaction", func(done Done) {
				wg := new(sync.WaitGroup)
				wg.Add(1)
				go func() {
					defer wg.Done()
					By(fmt.Sprintf("UAC waits %s", ok.Short()))
					msg := <-tpl.OutMsgs
					Expect(msg).ToNot(BeNil())
					Expect(msg.String()).To(Equal(ok.String()))
				}()

				By(fmt.Sprintf("UAS sends %s", ok.Short()))
				Expect(txl.Send(ok)).To(Succeed())

				wg.Wait()
				close(done)
			})

			Context("after 2xx Ok was sent", func() {
				BeforeEach(func() {
					go func() {
						By(fmt.Sprintf("UAS sends %s", ok.Short()))
						Expect(txl.Send(ok)).To(Succeed())
					}()
					go func() {
						By(fmt.Sprintf("UAC waits %s", ok.Short()))
						msg := <-tpl.OutMsgs
						Expect(msg).ToNot(BeNil())
						Expect(msg.String()).To(Equal(ok.String()))

						time.Sleep(time.Millisecond)
						By(fmt.Sprintf("UAC sends %s", ack.Short()))
						tpl.InMsgs <- ack
					}()
				})

				It("should receive ACK in separate transaction", func(done Done) {
					By(fmt.Sprintf("UAS receives %s", ack.Short()))
					msg := <-txl.Messages()
					Expect(msg).ToNot(BeNil())
					Expect(msg.String()).To(Equal(ack.String()))
					Expect(msg.Tx()).ToNot(BeNil())
					Expect(msg.Tx().Key()).To(Equal(ackTxKey))

					close(done)
				})
			})

			Context("after 3xx was sent", func() {
				BeforeEach(func() {
					go func() {
						By(fmt.Sprintf("UAS sends %s", notOk.Short()))
						Expect(txl.Send(notOk)).To(Succeed())
					}()
					go func() {
						By(fmt.Sprintf("UAC waits %s", notOk.Short()))
						msg := <-tpl.OutMsgs
						Expect(msg).ToNot(BeNil())
						Expect(msg.String()).To(Equal(notOk.String()))

						time.Sleep(time.Millisecond)
						By(fmt.Sprintf("UAC sends %s", notOkAck.Short()))
						tpl.InMsgs <- notOkAck
					}()
				})
			})
		})

		PIt("should open server tx, send 100 Trying response, then 200 OK", func(done Done) {
			wg := new(sync.WaitGroup)
			wg.Add(1)
			go func() {
				defer wg.Done()

				var msg core.Message
				By(fmt.Sprintf("UAC sends %s", invite.Short()))
				tpl.InMsgs <- invite

				By(fmt.Sprintf("UAC waits %s", trying.Short()))
				msg = <-tpl.OutMsgs
				Expect(msg).ToNot(BeNil())
				Expect(msg.String()).To(Equal(trying.String()))

				By(fmt.Sprintf("UAC waits %s", ok.Short()))
				msg = <-tpl.OutMsgs
				Expect(msg).ToNot(BeNil())
				Expect(msg.String()).To(Equal(ok.String()))

				By(fmt.Sprintf("UAC sends %s", ack.Short()))
				tpl.InMsgs <- ack
			}()

			var msg transaction.TxMessage

			By(fmt.Sprintf("UAS receives %s", invite.Short()))
			msg = <-txl.Messages()
			Expect(msg).ToNot(BeNil())
			Expect(msg.String()).To(Equal(invite.String()))
			Expect(msg.Tx()).ToNot(BeNil())
			Expect(msg.Tx().Key()).To(Equal(inviteTxKey))

			By(fmt.Sprintf("UAS send %s", trying.Short()))
			time.Sleep(time.Second)

			By(fmt.Sprintf("UAS sends %s", ok.Short()))
			Expect(txl.Send(ok)).To(Succeed())

			By(fmt.Sprintf("UAS receives %s", ack.Short()))
			msg = <-txl.Messages()
			Expect(msg).ToNot(BeNil())
			Expect(msg.String()).To(Equal(ack.String()))
			Expect(msg.Tx()).ToNot(BeNil())
			Expect(msg.Tx().Key()).To(Equal(ackTxKey))

			wg.Wait()
			close(done)
		}, 3)
	})
})
