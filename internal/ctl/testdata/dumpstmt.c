/* dumpstmt.c <file>: parse.c's own statement stream, one per line, so the Go
   port can be diffed against it over the corpus. A statement may contain a
   newline only inside a string, so the separator is safe enough for a diff;
   \n inside a statement is printed as \\n. */
#include <stdio.h>
#include "parse.h"

int main(int argc, char *argv[])
{
  FILE *fp;
  char *s;
  int n = 0;
  if (argc != 2) { fprintf(stderr, "usage: dumpstmt <file>\n"); return 2; }
  fp = fopen(argv[1], "r");
  if (!fp) { perror(argv[1]); return 2; }
  while ((s = get_next_stmt(fp)) != NULL) {
    char *p;
    printf("%d\t", ++n);
    for (p = s; *p; p++) {
      if (*p == '\n') printf("\\n");
      else if (*p == '\\') printf("\\\\");
      else putchar(*p);
    }
    putchar('\n');
  }
  fclose(fp);
  return 0;
}
