��    )      d  ;   �      �  �   �  �   u  �   .  c   �     1     J  R  W  N   �  &   �  O         p  #   ~  !   �  *   �  D   �  @   4	  %   u	  &   �	  &   �	  (   �	     
     -
     9
  (   G
  '   p
  4   �
  4   �
  &     /   )  /   Y  7   �  -   �  %   �  %     "   ;     ^  .   t  #   �  '   �     �  �  �  �   �  �   �  �   v  q   8     �     �  �  �  _   �  +   �  f          "   �     �  8   �  Y     O   g  6   �  0   �  /     5   O  '   �     �     �  8   �  @     @   H  @   �  ,   �  7   �  7   /  C   g  6   �  3   �  2     (   I      r  7   �  /   �  0   �     ,                	   (                                 "          
                                     &                  '                                          #          !          )             %   $          --usestd3asciirules   Enable STD3 ASCII rules
      --no-alabelroundtrip  Disable A-label roundtrip for lookups
      --debug               Print debugging information
      --quiet               Silent operation
   -T, --tr46t               Enable TR46 transitional processing
  -N, --tr46nt              Enable TR46 non-transitional processing
      --no-tr46             Disable TR46 processing
   -d, --decode              Decode (punycode) domain name
  -l, --lookup              Lookup domain name (default)
  -r, --register            Register label
   -h, --help                Print help and exit
  -V, --version             Print version and exit
 A-label roundtrip failed Charset: %s
 Command line interface to the Libidn2 implementation of IDNA2008.

All strings are expected to be encoded in the locale charset.

To process a string that starts with `-', for example `-foo', use `--'
to signal the end of parameters, as in `idn2 --quiet -- -foo'.

Mandatory arguments to long options are mandatory for short options too.
 Internationalized Domain Name (IDNA2008) convert STRINGS, or standard input.

 Try `%s --help' for more information.
 Type each input string on a line by itself, terminated by a newline character.
 Unknown error Usage: %s [OPTION]... [STRINGS]...
 could not convert string to UTF-8 could not determine locale encoding format domain label has character forbidden in non-transitional mode (TR46) domain label has character forbidden in transitional mode (TR46) domain label has forbidden dot (TR46) domain label longer than 63 characters domain name longer than 255 characters input A-label and U-label does not match input A-label is not valid input error out of memory punycode conversion resulted in overflow punycode encoded data will be too large string contains a context-j character with null rule string contains a context-o character with null rule string contains a disallowed character string contains a forbidden context-j character string contains a forbidden context-o character string contains a forbidden leading combining character string contains forbidden two hyphens pattern string contains invalid punycode data string contains unassigned code point string could not be NFC normalized string encoding error string has forbidden bi-directional properties string is not in Unicode NFC format string start/ends with forbidden hyphen success Project-Id-Version: libidn2 2.3.1
Report-Msgid-Bugs-To: bug-libidn2@gnu.org
PO-Revision-Date: 2022-03-10 09:34-0600
Last-Translator: Cristian Othón Martínez Vera <cfuga@cfuga.mx>
Language-Team: Spanish <es@tp.org.es>
Language: es
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8
Content-Transfer-Encoding: 8bit
X-Bugs: Report translation errors to the Language-Team address.
Plural-Forms: nplurals=2; plural=(n != 1);
       --usestd3asciirules   Activa las reglas ASCII STD3
      --no-alabelroundtip   Desactiva la vuelta de etiqueta-A para las búsquedas
      --debug               Muestra información de depuración
      --quiet               Operación silenciosa
   -T, --tr46t               Activa el procesamiento TR46 transicional
  -N, --tr46nt              Activa el procesamiento TR46 que no es transicional
      --no-tr46             Desactiva el procesamiento TR46
   -d, --decode              Decodifica el nombre de dominio (código puny)
  -l, --lookup              Busca el nombre de dominio (por defecto)
  -r, --register            Etiqueta de registro
   -h, --help                Muestra la ayuda y termina
  -V, --version             Muestra la versión y termina
 falló la vuelta de etiqueta-A Conjunto de caracteres: %s
 Interfaz de línea de órdenes para la implementación Libidn2 de IDNA2008.

Se espera que todas las cadenas estén codificadas en el conjunto de
caracteres local.

Para procesar una cadena que inicia con '-', por ejemplo '-foo', use '--'
para indicar el final de los parámetros, como en `idn2 --quiet -a -- -foo'.

Los argumentos obligatorios para las opciones largas son también obligatorias
para las opciones cortas.
 Nombres de Dominio Internacionalizados (IDNA2008) convierte CADENAS, o la entrada estándard.

 Pruebe `%s --help' para más información.
 Escriba cada cadena de entrada en una línea para cada uno, terminando con carácter de línea nueva.
 Error desconocido Uso: %s [OPCIÓN]... [CADENAS]...
 no se puede la cadena a a UTF-8 no se puede determinar el formato de codificación local la etiqueta de dominio tiene un carácter prohibido en modo que no es transicional (TR46) la etiqueta de dominio tiene un carácter prohibido en modo transicional (TR46) la etiqueta de dominio tiene un punto prohibido (TR46) etiqueta de dominio más largo que 63 caracteres nombre de dominio más largo que 255 caracteres no coinciden las entradas con etiqueta-A y etiqueta-U la entrada con etiqueta-A no es válida error de entrada memoria agotada la conversión de código puny resulta en desbordamiento los datos codificados con código puny serían demasiado grandes la cadena contiene un carácter de contexto-j con una regla nula la cadena contiene on carácter de contexto-o con una regla nula la cadena contiene un carácter no permitido la cadena contiene un carácter de contexto-j prohibido la cadena contiene un carácter de contexto-o prohibido la cadena contiene una combinación inicial de caracteres prohibida la cadena contiene un patrón prohibido de dos guiones la cadena contiene datos de código puny inválidos la cadena contiene un punto de código sin asignar no se puede normalizar la cadena con NFC error de codificación de cadena la cadena tiene propiedades bi-direccionales prohibidas la cadena no está en el formato NFC de Unicode la cadena inicia/termina con un guión prohibido éxito 