/*
 * qablocked <db>: is this transaction waiting on a lock?
 *
 * The one thing the isolation controller cannot ask in Go. `MC: wait until C2
 * blocked;` -- the statement the whole .ctl language exists for -- is answered
 * from the server's lock table through tran_is_blocked(), a symbol libcubridcs
 * exports and no header declares, which ctltool's qactl.c has always called
 * (cubrid_drv.c:50, :594). Everything else the controller does is process and
 * pipe work, and that is the runner's.
 *
 * It prints "ready" once it is connected, then reads a line at a time:
 *
 *   <n>     a transaction index -- answered with 1 or 0
 *   dump    the lock table -- answered with its length, then that many bytes
 *
 * The dump is the other thing the controller cannot write itself: qactl prints
 * it when a `wait until` command has failed (qactl.c:2271), and it lands in the
 * case's result. It is built against the engine under test, as ctltool is, because the
 * symbol is the engine's.
 */

#include <stdio.h>
#include <stdlib.h>

#include "dbi.h"

extern bool tran_is_blocked (int tran_index);
/* Two arguments, not one. ctltool declares this with one (cubrid_drv.c:49) and
 * calls it with one (qactl.c:2271), so what its dump contains is decided by
 * whatever is in the register the second argument would have come in: zero and
 * the whole lock table is printed, non-zero and only the contended resources
 * are. Measured: qactl printed 126 lines where this printed 43, until the
 * declaration was fixed here (evidence/spec-corrections.md §9). */
extern void lock_dump (FILE * outfp, int is_contention);

int
main (int argc, char *argv[])
{
  char line[64];
  int db_error;

  if (argc != 2)
    {
      fprintf (stderr, "usage: qablocked <db>\n");
      return 2;
    }

  /* "qactl" is the name the server records for this connection, and it is what
   * the lock table shows for the controller's own transaction. The controller
   * is what qactl was; a dump that called it something else would read as an
   * extra client. */
  db_error = db_restart ("qactl", 0, argv[1]);
  if (db_error != NO_ERROR)
    {
      /* The caller decides what to do about it -- qactl.c:3038 starts the
       * server and tries again, because recovery cases kill it. */
      fprintf (stderr, "qablocked: cannot connect to %s: %d\n", argv[1],
	       db_error);
      return 1;
    }

  /* The answers must not sit in a buffer: the controller is waiting for each
   * one before it sends the next question. */
  setvbuf (stdout, NULL, _IONBF, 0);

  /* Say so, so that the controller knows the database answered rather than
   * having to guess from a process that has not died yet. */
  printf ("ready\n");

  while (fgets (line, sizeof (line), stdin) != NULL)
    {
      if (line[0] == 'd')
	{
	  /* The length first, because the dump is many lines and the reader
	   * has no other way to know where it ends. */
	  char *buf = NULL;
	  size_t len = 0;
	  FILE *ms = open_memstream (&buf, &len);
	  if (ms == NULL)
	    {
	      printf ("0\n");
	      continue;
	    }
	  lock_dump (ms, 0);
	  fclose (ms);
	  printf ("%lu\n", (unsigned long) len);
	  fwrite (buf, 1, len, stdout);
	  free (buf);
	}
      else
	{
	  printf ("%d\n", tran_is_blocked (atoi (line)) ? 1 : 0);
	}
    }

  db_shutdown ();
  return 0;
}
